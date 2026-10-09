package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
)

// statsToday is today in these tests, a Saturday
var statsToday = statsDate("2026-10-10")

// statsDate is a calendar date the way a stat_date reads back: midnight UTC
func statsDate(date string) time.Time {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return t
}

// statsDay is a day's counts, with five characters a word
func statsDay(date string, messages, words int) db.RoomDailyStats {
	return db.RoomDailyStats{StatDate: statsDate(date), MessageCount: messages, WordCount: words, CharacterCount: words * 5}
}

var statsMembers = newStatsPeople([]RoomMember{
	{UserID: "@alice:example.com", Name: "Alice Smith"},
	{UserID: "@bob:example.com", Name: "Bob"},
	{UserID: "@siikabot:example.com", Name: "Siikabot", IsBot: true},
})

var statsCountedSince = time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)

var statsTotals = db.RoomMessageTotals{
	PeopleCount: 3, MessageCount: 200, WordCount: 1000, CharacterCount: 6000, CountedSince: &statsCountedSince,
}

func TestParseStatsRequest(t *testing.T) {
	// People default to the author of the latest message, as !stats does without an argument
	request, err := parseStatsRequest(statsArgs{Kind: "users"}, "@alice:example.com", statsToday)
	if err != nil || !slices.Equal(request.userIDs, []string{"@alice:example.com"}) {
		t.Errorf("users without user IDs = %v, %v", request.userIDs, err)
	}

	request, err = parseStatsRequest(statsArgs{Kind: " Users ", UserIDs: []string{"@bob:example.com", " @bob:example.com ", "@carol:example.com"}},
		"@alice:example.com", statsToday)
	if err != nil || !slices.Equal(request.userIDs, []string{"@bob:example.com", "@carol:example.com"}) {
		t.Errorf("users weren't trimmed and deduplicated: %v, %v", request.userIDs, err)
	}

	request, err = parseStatsRequest(statsArgs{Kind: "top"}, "", statsToday)
	if err != nil || request.rankBy != db.RankByMessages || request.limit != statsDefaultTop {
		t.Errorf("top without options = %+v, %v", request, err)
	}
	request, err = parseStatsRequest(statsArgs{Kind: "top", OrderBy: "Words", Limit: 100}, "", statsToday)
	if err != nil || request.rankBy != db.RankByWords || request.limit != statsMaxTop {
		t.Errorf("top by words, too many = %+v, %v", request, err)
	}

	tests := []struct {
		name     string
		args     statsArgs
		from, to string
	}{
		{"the 30 days up to today", statsArgs{Kind: "activity"}, "2026-09-11", "2026-10-10"},
		{"one day", statsArgs{Kind: "activity", Date: "2026-03-01", Days: 1}, "2026-03-01", "2026-03-01"},
		{"a week up to a date", statsArgs{Kind: "activity", Date: "2026-03-07", Days: 7}, "2026-03-01", "2026-03-07"},
		{"too many days", statsArgs{Kind: "activity", Days: 1000}, "2025-10-10", "2026-10-10"},
	}
	for _, tt := range tests {
		request, err := parseStatsRequest(tt.args, "", statsToday)
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if got := request.from.Format(time.DateOnly) + " " + request.to.Format(time.DateOnly); got != tt.from+" "+tt.to {
			t.Errorf("%s: the period is %s, want %s %s", tt.name, got, tt.from, tt.to)
		}
	}
}

func TestParseStatsRequestRefusesBadInput(t *testing.T) {
	var eleven []string
	for i := range 11 {
		eleven = append(eleven, fmt.Sprintf("@member%02d:example.com", i))
	}

	for name, args := range map[string]statsArgs{
		"an unknown kind":      {Kind: "everything"},
		"a name":               {Kind: "users", UserIDs: []string{"alice"}},
		"no server":            {Kind: "users", UserIDs: []string{"@alice"}},
		"an empty user ID":     {Kind: "users", UserIDs: []string{" "}},
		"too many people":      {Kind: "users", UserIDs: eleven},
		"an unknown ranking":   {Kind: "top", OrderBy: "characters"},
		"a date in a mess":     {Kind: "activity", Date: "10.10.2026"},
		"a date in the future": {Kind: "activity", Date: "2026-10-11"},
	} {
		if _, err := parseStatsRequest(args, "@alice:example.com", statsToday); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// Without user IDs or an author, there is no one to look up
	if _, err := parseStatsRequest(statsArgs{Kind: "users"}, "", statsToday); err == nil {
		t.Error("users without user IDs or an author was accepted")
	}
}

// The schema the model is given offers the kinds and rankings the handler accepts
func TestMessageStatsToolSchema(t *testing.T) {
	var schema struct {
		Properties struct {
			Kind    struct{ Enum []string } `json:"kind"`
			OrderBy struct{ Enum []string } `json:"order_by"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(MessageStatsToolDefinition.Function.Parameters, &schema); err != nil {
		t.Fatalf("the parameters aren't JSON: %v", err)
	}
	if !slices.Equal(schema.Properties.Kind.Enum, statsKinds) {
		t.Errorf("the schema offers the kinds %v, the handler takes %v", schema.Properties.Kind.Enum, statsKinds)
	}
	if want := []string{db.RankByMessages, db.RankByWords}; !slices.Equal(schema.Properties.OrderBy.Enum, want) {
		t.Errorf("the schema offers the rankings %v, the handler takes %v", schema.Properties.OrderBy.Enum, want)
	}
}

// A call is checked before anything is looked up, and these all fail before a database is needed
func TestMessageStatsToolCallChecksBeforeLookingUp(t *testing.T) {
	ctx := context.WithValue(context.Background(), "room_id", "!room:example.com")
	for name, arguments := range map[string]string{
		"arguments that aren't JSON": `not json`,
		"an unknown kind":            `{"kind": "everything"}`,
		"a name for a user ID":       `{"kind": "users", "user_ids": ["alice"]}`,
		"users without an author":    `{"kind": "users"}`,
	} {
		if _, err := handleMessageStatsToolCall(ctx, arguments); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// Without the room the turn happens in, there is nothing to look up
	if _, err := handleMessageStatsToolCall(context.Background(), `{"kind": "top"}`); err == nil {
		t.Error("a lookup without a room succeeded")
	}
}

func TestFormatUserStats(t *testing.T) {
	// Three hours ahead of UTC, which moves Alice's last message past midnight
	loc := time.FixedZone("UTC+3", 3*60*60)
	alice := db.RankedMessageStats{
		MessageStats: db.MessageStats{
			UserID: "@alice:example.com", MessageCount: 50, WordCount: 400, CharacterCount: 2000,
			FirstSeen: time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC), LastSeen: time.Date(2026, 10, 9, 22, 30, 0, 0, time.UTC),
		},
		Rank: 2,
	}
	carol := db.RankedMessageStats{
		MessageStats: db.MessageStats{
			UserID: "@carol:example.com", MessageCount: 1, WordCount: 1, CharacterCount: 5,
			FirstSeen: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), LastSeen: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		},
		Rank: 3,
	}

	asked := []string{"@alice:example.com", "@carol:example.com", "@dave:example.com", "@siikabot:example.com"}
	got := formatUserStats(asked, []db.RankedMessageStats{carol, alice}, statsTotals, statsMembers, loc)

	want := "Counting text messages since 2026-01-31, 3 people have sent 200 messages, 1000 words and 6000 characters in this room.\n" +
		"- Alice Smith (@alice:example.com): 50 messages (25.0% of the room's, #2 of 3), 400 words and 2000 characters, " +
		"8.0 words and 40.0 characters a message. First counted 2026-02-01 11:00, last message 2026-10-10 01:30.\n" +
		"- @carol:example.com (not in the room): 1 message (0.5% of the room's, #3 of 3), 1 word and 5 characters, " +
		"1.0 words and 5.0 characters a message. First counted 2026-03-01 13:00, last message 2026-03-01 13:00.\n" +
		"- @dave:example.com (not in the room): no messages counted.\n" +
		"- Siikabot (@siikabot:example.com) (you): your own messages aren't counted."
	if got != want {
		t.Errorf("formatUserStats() =\n%s\nwant\n%s", got, want)
	}

	// Without the room's members, people are shown by user ID
	got = formatUserStats([]string{"@alice:example.com"}, []db.RankedMessageStats{alice}, statsTotals, newStatsPeople(nil), loc)
	assertContains(t, got, "\n- @alice:example.com: 50 messages")
}

func TestFormatTopStats(t *testing.T) {
	stats := []db.RankedMessageStats{
		{MessageStats: db.MessageStats{UserID: "@bob:example.com", MessageCount: 120, WordCount: 300}, Rank: 1},
		{MessageStats: db.MessageStats{UserID: "@alice:example.com", MessageCount: 40, WordCount: 500}, Rank: 2},
		{MessageStats: db.MessageStats{UserID: "@carol:example.com", MessageCount: 40, WordCount: 200}, Rank: 2},
	}

	got := formatTopStats(stats, statsTotals, db.RankByMessages, statsMembers, time.UTC)
	want := "Counting text messages since 2026-01-31, 3 people have sent 200 messages, 1000 words and 6000 characters in this room. " +
		"The ones with the most messages:\n" +
		"1. Bob (@bob:example.com): 120 messages (60.0%), 300 words\n" +
		"2. Alice Smith (@alice:example.com): 40 messages (20.0%), 500 words\n" +
		"2. @carol:example.com (not in the room): 40 messages (20.0%), 200 words"
	if got != want {
		t.Errorf("formatTopStats() =\n%s\nwant\n%s", got, want)
	}

	// Ranked by words, the words come first, with their share
	byWords := []db.RankedMessageStats{{MessageStats: db.MessageStats{UserID: "@alice:example.com", MessageCount: 40, WordCount: 500}, Rank: 1}}
	assertContains(t, formatTopStats(byWords, statsTotals, db.RankByWords, statsMembers, time.UTC),
		"The ones with the most words:\n1. Alice Smith (@alice:example.com): 500 words (50.0%), 40 messages")
}

func TestFormatStatsForARoomWithNothingCounted(t *testing.T) {
	if got := formatUserStats([]string{"@alice:example.com"}, nil, db.RoomMessageTotals{}, statsMembers, time.UTC); got != noStatsYet {
		t.Errorf("formatUserStats() = %q", got)
	}
	if got := formatTopStats(nil, db.RoomMessageTotals{}, db.RankByMessages, statsMembers, time.UTC); got != noStatsYet {
		t.Errorf("formatTopStats() = %q", got)
	}
	if got := formatActivity(nil, nil, statsDate("2026-09-11"), statsToday, statsToday); got != noStatsYet {
		t.Errorf("formatActivity() = %q", got)
	}
}

// A period up to 31 days long is shown day by day, with the days on which nothing was counted
func TestFormatActivityByDay(t *testing.T) {
	first := statsDate("2026-02-06")
	days := []db.RoomDailyStats{statsDay("2026-10-04", 10, 50), statsDay("2026-10-06", 30, 100), statsDay("2026-10-10", 2, 8)}

	got := formatActivity(days, &first, statsDate("2026-10-04"), statsToday, statsToday)
	want := "Text messages in this room per day, from 2026-10-04 to 2026-10-10: 42 messages, 158 words and 790 characters in all, " +
		"6.0 messages a day. The busiest day was Tue 2026-10-06, with 30 messages.\n" +
		"- Sun 2026-10-04: 10 messages, 50 words\n" +
		"- Mon 2026-10-05: no messages\n" +
		"- Tue 2026-10-06: 30 messages, 100 words\n" +
		"- Wed 2026-10-07: no messages\n" +
		"- Thu 2026-10-08: no messages\n" +
		"- Fri 2026-10-09: no messages\n" +
		"- Sat 2026-10-10 (today, so far): 2 messages, 8 words"
	if got != want {
		t.Errorf("formatActivity() =\n%s\nwant\n%s", got, want)
	}
}

// A longer period is shown month by month, and a month it covers only part of says so
func TestFormatActivityByMonth(t *testing.T) {
	first := statsDate("2026-02-06")
	days := []db.RoomDailyStats{
		statsDay("2026-08-20", 5, 20), statsDay("2026-09-01", 10, 40), statsDay("2026-09-30", 20, 60), statsDay("2026-10-02", 3, 9),
	}

	got := formatActivity(days, &first, statsDate("2026-08-11"), statsToday, statsToday)
	want := "Text messages in this room per month, from 2026-08-11 to 2026-10-10: 38 messages, 129 words and 645 characters in all, " +
		"0.6 messages a day. The busiest day was Wed 2026-09-30, with 20 messages.\n" +
		"- 2026-08 (21 of 31 days): 5 messages, 20 words\n" +
		"- 2026-09: 30 messages, 100 words\n" +
		"- 2026-10 (10 of 31 days): 3 messages, 9 words"
	if got != want {
		t.Errorf("formatActivity() =\n%s\nwant\n%s", got, want)
	}
}

// Days before anything was counted in the room are left out rather than shown as quiet
func TestFormatActivityStartsWhenCountingBegan(t *testing.T) {
	first := statsDate("2026-10-08")

	got := formatActivity([]db.RoomDailyStats{statsDay("2026-10-08", 4, 12)}, &first, statsDate("2026-09-11"), statsToday, statsToday)
	want := "Nothing was counted in this room before 2026-10-08, so the period starts then.\n" +
		"Text messages in this room per day, from 2026-10-08 to 2026-10-10: 4 messages, 12 words and 60 characters in all, " +
		"1.3 messages a day. The busiest day was Thu 2026-10-08, with 4 messages.\n" +
		"- Thu 2026-10-08: 4 messages, 12 words\n" +
		"- Fri 2026-10-09: no messages\n" +
		"- Sat 2026-10-10 (today, so far): no messages"
	if got != want {
		t.Errorf("formatActivity() =\n%s\nwant\n%s", got, want)
	}

	got = formatActivity(nil, &first, statsDate("2026-09-01"), statsDate("2026-09-30"), statsToday)
	if want := "Nothing was counted in this room before 2026-10-08, the first day anything was counted here."; got != want {
		t.Errorf("a period before counting began = %q", got)
	}
}

func TestFormatActivityForOneDayOrAQuietPeriod(t *testing.T) {
	first := statsDate("2026-02-06")

	got := formatActivity([]db.RoomDailyStats{statsDay("2026-10-10", 12, 80)}, &first, statsToday, statsToday, statsToday)
	if want := "Text messages in this room on Sat 2026-10-10 (today, so far): 12 messages, 80 words and 400 characters."; got != want {
		t.Errorf("today = %q", got)
	}

	got = formatActivity(nil, &first, statsDate("2026-10-09"), statsDate("2026-10-09"), statsToday)
	if want := "Text messages in this room on Fri 2026-10-09: no messages."; got != want {
		t.Errorf("a quiet day = %q", got)
	}

	got = formatActivity(nil, &first, statsDate("2026-10-01"), statsDate("2026-10-07"), statsToday)
	if want := "No text messages were counted in this room from 2026-10-01 to 2026-10-07."; got != want {
		t.Errorf("a quiet week = %q", got)
	}
	if strings.Contains(got, "busiest") {
		t.Error("a quiet week names a busiest day")
	}
}
