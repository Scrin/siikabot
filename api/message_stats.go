package api

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// defaultStatsDays is the activity period used when the request does not ask for one
const defaultStatsDays = 30

// maxStatsDays bounds the activity period of a room
const maxStatsDays = 366

// maxActivityDays bounds the activity period across rooms, which counts chat turns too: usage
// records are pruned beyond this
const maxActivityDays = maxUsageDays

// statsTopPeople is how many people a room's leaderboard lists
const statsTopPeople = 10

// DayCounts is what was counted on one day
type DayCounts struct {
	// Date is the calendar date in the bot's time zone, YYYY-MM-DD
	Date     string `json:"date"`
	Messages int    `json:"messages"`
	Words    int    `json:"words"`
}

// ActivityResponse is the text messages counted per day over a period, up to today
type ActivityResponse struct {
	Days int `json:"days"`
	// From and To are the first and last day shown, YYYY-MM-DD. From is later than the start of the
	// period when counting began later, since the days before it weren't quiet, only not counted.
	From string `json:"from"`
	To   string `json:"to"`
	// FirstDay is the first day anything was counted on, null if nothing has been
	FirstDay   *string `json:"first_day"`
	Messages   int     `json:"messages"`
	Words      int     `json:"words"`
	Characters int     `json:"characters"`
	// Busiest is the day with the most messages, null if nothing was counted in the period
	Busiest *DayCounts `json:"busiest"`
	// Daily has every day shown, including the ones on which nothing was counted
	Daily []DayCounts `json:"daily"`
}

// PersonStats is someone's message counts in a room, since counting began
type PersonStats struct {
	UserID string `json:"user_id"`
	// DisplayName is their display name in the room, left out if they have none or aren't in it
	DisplayName string `json:"display_name,omitempty"`
	// InRoom says they are a member of the room now
	InRoom bool `json:"in_room"`
	// Rank is their place by messages. People with the same count share a place.
	Rank     int `json:"rank"`
	Messages int `json:"messages"`
	Words    int `json:"words"`
	// Share is their share of the room's messages, from 0 to 1
	Share       float64   `json:"share"`
	LastMessage time.Time `json:"last_message"`
}

// PeopleResponse is the counts of everyone counted in a room, since counting began
type PeopleResponse struct {
	// CountedSince is when the earliest counted message was sent, null if none has been
	CountedSince *time.Time `json:"counted_since"`
	PeopleCount  int        `json:"people_count"`
	Messages     int        `json:"messages"`
	Words        int        `json:"words"`
	Characters   int        `json:"characters"`
	// Top are the people with the most messages, most first
	Top []PersonStats `json:"top"`
	// Me is the viewer's own counts, null if nothing of theirs has been counted in the room
	Me *PersonStats `json:"me"`
}

// RoomStatsResponse is the message stats of a room: its activity over a period, and the counts of
// its people, which have no breakdown by date
type RoomStatsResponse struct {
	RoomID   string           `json:"room_id"`
	Activity ActivityResponse `json:"activity"`
	People   PeopleResponse   `json:"people"`
}

// RoomActivity is one room's counts over a period
type RoomActivity struct {
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name,omitempty"`
	Messages int    `json:"messages"`
	Words    int    `json:"words"`
	// Share is the room's share of the messages counted in every room, from 0 to 1
	Share float64 `json:"share"`
	// ChatTurns is how many messages the bot handled as chat in the room
	ChatTurns int `json:"chat_turns"`
}

// MessageActivityResponse is the activity across every room over a period
type MessageActivityResponse struct {
	Activity  ActivityResponse `json:"activity"`
	ChatTurns int              `json:"chat_turns"`
	// Rooms are the rooms with anything counted or any chat turns in the period, most messages first
	Rooms []RoomActivity `json:"rooms"`
}

// RoomStatsHandler returns the message stats of a room shared with the user. Like the room's
// members, only someone who has joined the room sees them.
// GET /api/rooms/:roomId/stats?days=30
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RoomStatsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing room ID"})
		return
	}
	days, ok := statsDays(c, maxStatsDays)
	if !ok {
		return
	}

	joinedRooms, err := db.FindJoinedRooms(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to verify room access")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to verify access"})
		return
	}
	if !slices.Contains(joinedRooms, roomID) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied to this room"})
		return
	}

	respondRoomStats(c, roomID, userID, days)
}

// AdminRoomStatsHandler returns the message stats of any room the bot counts messages in (admin
// only)
// GET /api/admin/rooms/:roomId/stats?days=30
func AdminRoomStatsHandler(c *gin.Context) {
	userID, _ := GetUserIDFromContext(c)

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing room ID"})
		return
	}
	days, ok := statsDays(c, maxStatsDays)
	if !ok {
		return
	}

	respondRoomStats(c, roomID, userID, days)
}

// respondRoomStats responds with a room's message stats, with the viewer's own counts among them
func respondRoomStats(c *gin.Context, roomID, viewerID string, days int) {
	ctx := c.Request.Context()
	today := db.StatDate(time.Now())

	fail := func(err error) {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Int("days", days).Msg("Failed to fetch room stats")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room stats"})
	}

	first, err := db.GetFirstRoomStatDate(ctx, roomID)
	if err != nil {
		fail(err)
		return
	}
	daily, err := db.GetRoomDailyStats(ctx, roomID, today.AddDate(0, 0, 1-days), today)
	if err != nil {
		fail(err)
		return
	}
	totals, err := db.GetRoomMessageTotals(ctx, roomID)
	if err != nil {
		fail(err)
		return
	}
	top, err := db.GetTopMessageStats(ctx, roomID, db.RankByMessages, statsTopPeople)
	if err != nil {
		fail(err)
		return
	}
	mine, err := db.GetRankedMessageStats(ctx, roomID, []string{viewerID})
	if err != nil {
		fail(err)
		return
	}
	members, err := db.GetJoinedMembers(ctx, roomID)
	if err != nil {
		fail(err)
		return
	}

	c.JSON(http.StatusOK, RoomStatsResponse{
		RoomID:   roomID,
		Activity: activityResponse(daily, first, days, today),
		People:   peopleResponse(totals, top, mine, members),
	})
}

// MessageActivityHandler returns the message activity across every room the bot counts messages
// in, with the chat turns in each (admin only)
// GET /api/admin/message-activity?days=30
func MessageActivityHandler(c *gin.Context) {
	ctx := c.Request.Context()

	days, ok := statsDays(c, maxActivityDays)
	if !ok {
		return
	}
	today := db.StatDate(time.Now())

	fail := func(err error) {
		log.Error().Ctx(ctx).Err(err).Int("days", days).Msg("Failed to fetch message activity")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch message activity"})
	}

	first, err := db.GetFirstStatDate(ctx)
	if err != nil {
		fail(err)
		return
	}
	daily, err := db.GetAllRoomsDailyStats(ctx, today.AddDate(0, 0, 1-days), today)
	if err != nil {
		fail(err)
		return
	}
	// The chat turns over the same days as the counts, from the start of the first day shown
	usage, err := db.GetChatUsageSummary(ctx, db.StatDayStart(shownFrom(first, days, today)))
	if err != nil {
		fail(err)
		return
	}

	rooms, chatTurns := roomActivities(daily, usage, func(roomID string) string {
		return matrix.GetRoomName(ctx, roomID)
	})
	c.JSON(http.StatusOK, MessageActivityResponse{
		Activity:  activityResponse(daily, first, days, today),
		ChatTurns: chatTurns,
		Rooms:     rooms,
	})
}

// statsDays reads the period asked for, in days. When it isn't a number from 1 to max, it
// responds with an error and returns false.
func statsDays(c *gin.Context, max int) (int, bool) {
	raw := c.Query("days")
	if raw == "" {
		return defaultStatsDays, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 || days > max {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "days must be between 1 and " + strconv.Itoa(max)})
		return 0, false
	}
	return days, true
}

// shownFrom is the first day shown of a period of days up to today: the start of the period, or
// the first counted day if that is later
func shownFrom(first *time.Time, days int, today time.Time) time.Time {
	from := today.AddDate(0, 0, 1-days)
	if first != nil && from.Before(*first) {
		return *first
	}
	return from
}

// activityResponse sums the daily counts of one room or many over the days of a period up to
// today, with a zero for each day on which nothing was counted. The days before the first counted
// day are left out rather than shown as quiet.
func activityResponse(rows []db.RoomDailyStats, first *time.Time, days int, today time.Time) ActivityResponse {
	from := shownFrom(first, days, today)
	response := ActivityResponse{Days: days, From: from.Format(time.DateOnly), To: today.Format(time.DateOnly), Daily: []DayCounts{}}
	if first == nil {
		return response
	}
	firstDay := first.Format(time.DateOnly)
	response.FirstDay = &firstDay

	index := make(map[string]int, days)
	for day := from; !day.After(today); day = day.AddDate(0, 0, 1) {
		index[day.Format(time.DateOnly)] = len(response.Daily)
		response.Daily = append(response.Daily, DayCounts{Date: day.Format(time.DateOnly)})
	}
	for _, row := range rows {
		i, ok := index[row.StatDate.Format(time.DateOnly)]
		if !ok {
			continue
		}
		response.Daily[i].Messages += row.MessageCount
		response.Daily[i].Words += row.WordCount
		response.Messages += row.MessageCount
		response.Words += row.WordCount
		response.Characters += row.CharacterCount
	}
	for _, day := range response.Daily {
		if day.Messages > 0 && (response.Busiest == nil || day.Messages > response.Busiest.Messages) {
			busiest := day
			response.Busiest = &busiest
		}
	}
	return response
}

// peopleResponse lists the people counted in a room by the names its members go by there, and
// says which of them aren't in it now
func peopleResponse(totals db.RoomMessageTotals, top, mine []db.RankedMessageStats, members []db.RoomMember) PeopleResponse {
	names := make(map[string]string, len(members))
	for _, member := range members {
		names[member.UserID] = ""
		if member.DisplayName != nil {
			names[member.UserID] = *member.DisplayName
		}
	}
	person := func(stats db.RankedMessageStats) PersonStats {
		name, inRoom := names[stats.UserID]
		return PersonStats{
			UserID:      stats.UserID,
			DisplayName: name,
			InRoom:      inRoom,
			Rank:        stats.Rank,
			Messages:    stats.MessageCount,
			Words:       stats.WordCount,
			Share:       share(stats.MessageCount, totals.MessageCount),
			LastMessage: stats.LastSeen,
		}
	}

	response := PeopleResponse{
		CountedSince: totals.CountedSince,
		PeopleCount:  totals.PeopleCount,
		Messages:     totals.MessageCount,
		Words:        totals.WordCount,
		Characters:   totals.CharacterCount,
		Top:          make([]PersonStats, 0, len(top)),
	}
	for _, stats := range top {
		response.Top = append(response.Top, person(stats))
	}
	if len(mine) > 0 {
		me := person(mine[0])
		response.Me = &me
	}
	return response
}

// roomActivities sums each room's counts and chat turns over a period, most messages first, and
// the chat turns in all of them
func roomActivities(rows []db.RoomDailyStats, usage []db.ChatUsageSummary, roomName func(roomID string) string) ([]RoomActivity, int) {
	byRoom := make(map[string]*RoomActivity)
	room := func(roomID string) *RoomActivity {
		if byRoom[roomID] == nil {
			byRoom[roomID] = &RoomActivity{RoomID: roomID}
		}
		return byRoom[roomID]
	}

	var messages, chatTurns int
	for _, row := range rows {
		r := room(row.RoomID)
		r.Messages += row.MessageCount
		r.Words += row.WordCount
		messages += row.MessageCount
	}
	for _, summary := range usage {
		room(summary.RoomID).ChatTurns += summary.Turns
		chatTurns += summary.Turns
	}

	rooms := make([]RoomActivity, 0, len(byRoom))
	for _, r := range byRoom {
		r.Share = share(r.Messages, messages)
		r.RoomName = roomName(r.RoomID)
		rooms = append(rooms, *r)
	}
	slices.SortFunc(rooms, func(a, b RoomActivity) int {
		return cmp.Or(cmp.Compare(b.Messages, a.Messages), cmp.Compare(b.ChatTurns, a.ChatTurns), cmp.Compare(a.RoomID, b.RoomID))
	})
	return rooms, chatTurns
}

// share is a part's share of a whole, from 0 to 1
func share(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
