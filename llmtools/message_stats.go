package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"maunium.net/go/mautrix/id"
)

// Kinds of message stats
const (
	statsKindUsers    = "users"
	statsKindTop      = "top"
	statsKindActivity = "activity"
)

var statsKinds = []string{statsKindUsers, statsKindTop, statsKindActivity}

// Limits on what one call covers. A number outside them is clamped rather than refused.
const (
	statsMaxUsers    = 10
	statsDefaultTop  = 10
	statsMaxTop      = 20
	statsDefaultDays = 30
	statsMaxDays     = 366
	// statsMaxDailyDays is the longest period shown day by day. A longer one is shown by month.
	statsMaxDailyDays = 31
)

// statsTimeLayout is how times are shown, the way the headers of the messages the model sees show
// them
const statsTimeLayout = "2006-01-02 15:04"

// noStatsYet is the answer for a room in which nothing has been counted
const noStatsYet = "No messages have been counted in this room yet."

// MessageStatsToolDefinition gives the message stats the bot keeps for the room a chat turn happens
// in: how much people have written there, never what.
//
// It sees the stats of that one room only. The room comes from the context the chat turn sets up,
// and the tool takes none of its own, so that no lookup reaches into another room the bot is in.
var MessageStatsToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name: "get_message_stats",
		Description: "Message statistics for this room, as counted by the bot: how many text messages, words and characters people have sent here. " +
			"Counts only, never what was said, and only for this room. Kinds: " +
			"users: totals for people, by default the author of the latest message. " +
			"top: the people who have sent the most. " +
			"activity: the room's messages per day over a period, or per month for a period over 31 days. " +
			"The totals per person cover everything since counting began, with no breakdown by date.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"kind": {
					"type": "string",
					"enum": ["users", "top", "activity"],
					"description": "What statistics to get"
				},
				"user_ids": {
					"type": "array",
					"items": {"type": "string"},
					"description": "users: Matrix user IDs, such as @alice:example.org, at most 10 (default the author of the latest message)"
				},
				"order_by": {
					"type": "string",
					"enum": ["messages", "words"],
					"description": "top: what to rank people by (default messages)"
				},
				"limit": {
					"type": "integer",
					"description": "top: how many people to list, 1-20 (default 10)"
				},
				"date": {
					"type": "string",
					"description": "activity: YYYY-MM-DD, the last day to include (default today)"
				},
				"days": {
					"type": "integer",
					"description": "activity: how many days up to date, 1-366 (default 30)"
				}
			},
			"required": ["kind"]
		}`),
	},
	Handler: handleMessageStatsToolCall,
	// Every message changes the counts
	ValidityDuration: 5 * time.Minute,
	Trusted:          true,
}

// statsArgs are the arguments of a tool call
type statsArgs struct {
	Kind    string   `json:"kind"`
	UserIDs []string `json:"user_ids"`
	OrderBy string   `json:"order_by"`
	Limit   int      `json:"limit"`
	Date    string   `json:"date"`
	Days    int      `json:"days"`
}

// statsRequest is a validated tool call, with defaults applied
type statsRequest struct {
	kind    string
	userIDs []string
	rankBy  string
	limit   int
	// from and to are the first and last day of an activity period, as midnight UTC on those
	// dates, which is how the daily counts read back
	from, to time.Time
}

func handleMessageStatsToolCall(ctx context.Context, arguments string) (string, error) {
	var args statsArgs

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received message stats tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse message stats tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	sender, _ := ctx.Value("sender").(string)
	today := db.StatDate(time.Now())
	request, err := parseStatsRequest(args, sender, today)
	if err != nil {
		return "", err
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("siikabot.message_stats.kind", request.kind))

	roomID, ok := ctx.Value("room_id").(string)
	if !ok || roomID == "" {
		log.Error().Ctx(ctx).Msg("No room in the tool context")
		return "", errors.New("the room isn't known")
	}
	// The room's members, when they are known, name the people in the stats
	members, _ := ctx.Value("room_members").([]RoomMember)
	people := newStatsPeople(members)

	switch request.kind {
	case statsKindUsers:
		return userStats(ctx, roomID, request.userIDs, people)
	case statsKindTop:
		return topStats(ctx, roomID, request.rankBy, request.limit, people)
	default:
		return roomActivity(ctx, roomID, request.from, request.to, today)
	}
}

// parseStatsRequest validates a tool call and applies the defaults: the people default to the
// author of the latest message, and the period to the days up to today
func parseStatsRequest(args statsArgs, sender string, today time.Time) (statsRequest, error) {
	request := statsRequest{kind: strings.ToLower(strings.TrimSpace(args.Kind))}
	if !slices.Contains(statsKinds, request.kind) {
		return request, fmt.Errorf("unknown kind %q, use one of: %s", args.Kind, strings.Join(statsKinds, ", "))
	}

	switch request.kind {
	case statsKindUsers:
		for _, userID := range args.UserIDs {
			userID = strings.TrimSpace(userID)
			if _, _, err := id.UserID(userID).Parse(); err != nil {
				return request, fmt.Errorf("%q is not a Matrix user ID, such as @alice:example.org; find_room_members finds someone by name", userID)
			}
			if !slices.Contains(request.userIDs, userID) {
				request.userIDs = append(request.userIDs, userID)
			}
		}
		if len(request.userIDs) > statsMaxUsers {
			return request, fmt.Errorf("at most %d people at a time, got %d", statsMaxUsers, len(request.userIDs))
		}
		if len(request.userIDs) == 0 {
			if sender == "" {
				return request, errors.New("user_ids is required, since the author of the latest message isn't known")
			}
			request.userIDs = []string{sender}
		}
	case statsKindTop:
		request.rankBy = strings.ToLower(strings.TrimSpace(args.OrderBy))
		if request.rankBy == "" {
			request.rankBy = db.RankByMessages
		}
		if request.rankBy != db.RankByMessages && request.rankBy != db.RankByWords {
			return request, fmt.Errorf("unknown order_by %q, use messages or words", args.OrderBy)
		}
		request.limit = clamp(args.Limit, statsDefaultTop, statsMaxTop)
	case statsKindActivity:
		request.to = today
		if date := strings.TrimSpace(args.Date); date != "" {
			parsed, err := time.Parse(time.DateOnly, date)
			if err != nil {
				return request, fmt.Errorf("date must be in YYYY-MM-DD format, got %q", date)
			}
			if parsed.After(today) {
				return request, fmt.Errorf("date can't be after today, %s", today.Format(time.DateOnly))
			}
			request.to = parsed
		}
		request.from = request.to.AddDate(0, 0, 1-clamp(args.Days, statsDefaultDays, statsMaxDays))
	}
	return request, nil
}

// userStats gets the message stats of people in the room
func userStats(ctx context.Context, roomID string, userIDs []string, people statsPeople) (string, error) {
	totals, err := db.GetRoomMessageTotals(ctx, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to get the room's message totals: %w", err)
	}
	stats, err := db.GetRankedMessageStats(ctx, roomID, userIDs)
	if err != nil {
		return "", fmt.Errorf("failed to get message stats: %w", err)
	}
	return formatUserStats(userIDs, stats, totals, people, botLocation()), nil
}

// topStats gets the people who have sent the most in the room
func topStats(ctx context.Context, roomID, rankBy string, limit int, people statsPeople) (string, error) {
	totals, err := db.GetRoomMessageTotals(ctx, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to get the room's message totals: %w", err)
	}
	stats, err := db.GetTopMessageStats(ctx, roomID, rankBy, limit)
	if err != nil {
		return "", fmt.Errorf("failed to get the top message stats: %w", err)
	}
	return formatTopStats(stats, totals, rankBy, people, botLocation()), nil
}

// roomActivity gets the room's counts over a period
func roomActivity(ctx context.Context, roomID string, from, to, today time.Time) (string, error) {
	first, err := db.GetFirstRoomStatDate(ctx, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to get the first counted day: %w", err)
	}
	days, err := db.GetRoomDailyStats(ctx, roomID, from, to)
	if err != nil {
		return "", fmt.Errorf("failed to get the daily counts: %w", err)
	}
	return formatActivity(days, first, from, to, today), nil
}

// statsPeople names the people in message stats the way the room knows them
type statsPeople struct {
	// members is nil when the room's members aren't known
	members map[string]RoomMember
}

func newStatsPeople(members []RoomMember) statsPeople {
	if members == nil {
		return statsPeople{}
	}
	byID := make(map[string]RoomMember, len(members))
	for _, member := range members {
		byID[member.UserID] = member
	}
	return statsPeople{members: byID}
}

// label is how someone is shown: the way the room's members are, and by user ID when they aren't
// in the room or its members aren't known
func (p statsPeople) label(userID string) string {
	if p.members == nil {
		return userID
	}
	member, ok := p.members[userID]
	if !ok {
		return userID + " (not in the room)"
	}
	return member.label()
}

// isBot reports whether someone is the bot, whose own messages aren't counted
func (p statsPeople) isBot(userID string) bool {
	return p.members[userID].IsBot
}

// totalsLine sums up what has been counted in the room
func totalsLine(totals db.RoomMessageTotals, loc *time.Location) string {
	return fmt.Sprintf("Counting text messages since %s, %s sent %s, %s and %s in this room.",
		totals.CountedSince.In(loc).Format(time.DateOnly), countOf(totals.PeopleCount, "person has", "people have"),
		countOf(totals.MessageCount, "message", "messages"), countOf(totals.WordCount, "word", "words"),
		countOf(totals.CharacterCount, "character", "characters"))
}

// formatUserStats renders the message stats of people in the room, in the order they were asked
// for. stats holds those of them who have anything counted.
func formatUserStats(userIDs []string, stats []db.RankedMessageStats, totals db.RoomMessageTotals, people statsPeople, loc *time.Location) string {
	if totals.CountedSince == nil {
		return noStatsYet
	}
	byID := make(map[string]db.RankedMessageStats, len(stats))
	for _, s := range stats {
		byID[s.UserID] = s
	}

	var b strings.Builder
	b.WriteString(totalsLine(totals, loc) + "\n")
	for _, userID := range userIDs {
		s, ok := byID[userID]
		switch {
		case ok:
			fmt.Fprintf(&b, "- %s: %s (%s of the room's, #%d of %d), %s and %s, %.1f words and %.1f characters a message. First counted %s, last message %s.\n",
				people.label(userID), countOf(s.MessageCount, "message", "messages"),
				percent(s.MessageCount, totals.MessageCount), s.Rank, totals.PeopleCount,
				countOf(s.WordCount, "word", "words"), countOf(s.CharacterCount, "character", "characters"),
				perMessage(s.WordCount, s.MessageCount), perMessage(s.CharacterCount, s.MessageCount),
				s.FirstSeen.In(loc).Format(statsTimeLayout), s.LastSeen.In(loc).Format(statsTimeLayout))
		case people.isBot(userID):
			fmt.Fprintf(&b, "- %s: your own messages aren't counted.\n", people.label(userID))
		default:
			fmt.Fprintf(&b, "- %s: no messages counted.\n", people.label(userID))
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// formatTopStats renders the people who have sent the most in the room, most first
func formatTopStats(stats []db.RankedMessageStats, totals db.RoomMessageTotals, rankBy string, people statsPeople, loc *time.Location) string {
	if totals.CountedSince == nil {
		return noStatsYet
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s The ones with the most %s:\n", totalsLine(totals, loc), rankBy)
	for _, s := range stats {
		messages, words := countOf(s.MessageCount, "message", "messages"), countOf(s.WordCount, "word", "words")
		if rankBy == db.RankByWords {
			fmt.Fprintf(&b, "%d. %s: %s (%s), %s\n", s.Rank, people.label(s.UserID), words, percent(s.WordCount, totals.WordCount), messages)
		} else {
			fmt.Fprintf(&b, "%d. %s: %s (%s), %s\n", s.Rank, people.label(s.UserID), messages, percent(s.MessageCount, totals.MessageCount), words)
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// formatActivity renders the room's counts over a period: day by day, or month by month for a
// period longer than statsMaxDailyDays. days holds the days of the period on which anything was
// counted, and first is the first day anything was counted on in the room, nil if nothing has been.
func formatActivity(days []db.RoomDailyStats, first *time.Time, from, to, today time.Time) string {
	if first == nil {
		return noStatsYet
	}
	if to.Before(*first) {
		return fmt.Sprintf("Nothing was counted in this room before %s, the first day anything was counted here.",
			first.Format(time.DateOnly))
	}

	var b strings.Builder
	if from.Before(*first) {
		fmt.Fprintf(&b, "Nothing was counted in this room before %s, so the period starts then.\n", first.Format(time.DateOnly))
		from = *first
	}

	byDate := make(map[string]db.RoomDailyStats, len(days))
	var total, busiest db.RoomDailyStats
	for _, day := range days {
		byDate[day.StatDate.Format(time.DateOnly)] = day
		addCounts(&total, day)
		if day.MessageCount > busiest.MessageCount {
			busiest = day
		}
	}

	if from.Equal(to) {
		fmt.Fprintf(&b, "Text messages in this room on %s: %s.", statsDayLabel(from, today), fullCounts(total))
		return b.String()
	}
	if total.MessageCount == 0 {
		fmt.Fprintf(&b, "No text messages were counted in this room from %s to %s.", from.Format(time.DateOnly), to.Format(time.DateOnly))
		return b.String()
	}

	dayCount := int(to.Sub(from)/(24*time.Hour)) + 1
	unit := "day"
	if dayCount > statsMaxDailyDays {
		unit = "month"
	}
	fmt.Fprintf(&b, "Text messages in this room per %s, from %s to %s: %s in all, %.1f messages a day. The busiest day was %s, with %s.\n",
		unit, from.Format(time.DateOnly), to.Format(time.DateOnly), fullCounts(total),
		float64(total.MessageCount)/float64(dayCount), statsDayLabel(busiest.StatDate, today),
		countOf(busiest.MessageCount, "message", "messages"))

	if dayCount <= statsMaxDailyDays {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			fmt.Fprintf(&b, "- %s: %s\n", statsDayLabel(day, today), activityCounts(byDate[day.Format(time.DateOnly)]))
		}
		return strings.TrimSuffix(b.String(), "\n")
	}

	// A month the period covers only part of says how much of it
	type month struct {
		start time.Time
		days  int
		stats db.RoomDailyStats
	}
	var months []month
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if len(months) == 0 || day.Day() == 1 {
			months = append(months, month{start: time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)})
		}
		current := &months[len(months)-1]
		current.days++
		addCounts(&current.stats, byDate[day.Format(time.DateOnly)])
	}
	for _, m := range months {
		label := m.start.Format("2006-01")
		if length := m.start.AddDate(0, 1, -1).Day(); m.days < length {
			label += fmt.Sprintf(" (%d of %d days)", m.days, length)
		}
		fmt.Fprintf(&b, "- %s: %s\n", label, activityCounts(m.stats))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// addCounts adds a day's counts to a sum
func addCounts(sum *db.RoomDailyStats, day db.RoomDailyStats) {
	sum.MessageCount += day.MessageCount
	sum.WordCount += day.WordCount
	sum.CharacterCount += day.CharacterCount
}

// statsDayLabel names a day, marking today, whose counts are still growing
func statsDayLabel(day, today time.Time) string {
	label := day.Format("Mon 2006-01-02")
	if day.Equal(today) {
		label += " (today, so far)"
	}
	return label
}

// fullCounts writes out every count of a day or a period
func fullCounts(stats db.RoomDailyStats) string {
	if stats.MessageCount == 0 {
		return "no messages"
	}
	return fmt.Sprintf("%s, %s and %s", countOf(stats.MessageCount, "message", "messages"),
		countOf(stats.WordCount, "word", "words"), countOf(stats.CharacterCount, "character", "characters"))
}

// activityCounts writes out the counts of a day or a month in a list of them
func activityCounts(stats db.RoomDailyStats) string {
	if stats.MessageCount == 0 {
		return "no messages"
	}
	return countOf(stats.MessageCount, "message", "messages") + ", " + countOf(stats.WordCount, "word", "words")
}

// countOf writes out a count of something
func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// percent is a part's share of a whole
func percent(part, whole int) string {
	if whole == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", float64(part)*100/float64(whole))
}

// perMessage averages a count over a number of messages
func perMessage(n, messages int) float64 {
	if messages == 0 {
		return 0
	}
	return float64(n) / float64(messages)
}
