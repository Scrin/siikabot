package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/gin-gonic/gin"
)

// statsToday is today in these tests
var statsToday = statsDate("2026-10-10")

// statsDate is a calendar date the way a stat_date reads back: midnight UTC
func statsDate(date string) time.Time {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return t
}

func dailyRow(roomID, date string, messages, words int) db.RoomDailyStats {
	return db.RoomDailyStats{RoomID: roomID, StatDate: statsDate(date), MessageCount: messages, WordCount: words, CharacterCount: words * 5}
}

// Every day of the period is there, a quiet one with zeros, so a chart of it has no gaps
func TestActivityResponseFillsQuietDays(t *testing.T) {
	first := statsDate("2026-02-06")
	rows := []db.RoomDailyStats{dailyRow("!room:x", "2026-10-05", 12, 60), dailyRow("!room:x", "2026-10-08", 30, 90)}

	response := activityResponse(rows, &first, 7, statsToday)

	if response.From != "2026-10-04" || response.To != "2026-10-10" || response.Days != 7 {
		t.Errorf("the period is %s to %s over %d days", response.From, response.To, response.Days)
	}
	if response.FirstDay == nil || *response.FirstDay != "2026-02-06" {
		t.Errorf("the first day is %v", response.FirstDay)
	}
	if len(response.Daily) != 7 {
		t.Fatalf("%d days, want 7: %+v", len(response.Daily), response.Daily)
	}
	if response.Daily[0] != (DayCounts{Date: "2026-10-04"}) || response.Daily[1] != (DayCounts{Date: "2026-10-05", Messages: 12, Words: 60}) {
		t.Errorf("the first days are %+v", response.Daily[:2])
	}
	if response.Messages != 42 || response.Words != 150 || response.Characters != 750 {
		t.Errorf("the totals are %d messages, %d words, %d characters", response.Messages, response.Words, response.Characters)
	}
	if response.Busiest == nil || *response.Busiest != (DayCounts{Date: "2026-10-08", Messages: 30, Words: 90}) {
		t.Errorf("the busiest day is %+v", response.Busiest)
	}
}

// Across rooms, a day's counts are the sum of every room's
func TestActivityResponseSumsRooms(t *testing.T) {
	first := statsDate("2026-02-06")
	rows := []db.RoomDailyStats{dailyRow("!a:x", "2026-10-10", 3, 10), dailyRow("!b:x", "2026-10-10", 4, 20)}

	response := activityResponse(rows, &first, 1, statsToday)
	if len(response.Daily) != 1 || response.Daily[0] != (DayCounts{Date: "2026-10-10", Messages: 7, Words: 30}) {
		t.Errorf("today is %+v", response.Daily)
	}
}

// The days before anything was counted weren't quiet, only not counted, so they aren't shown
func TestActivityResponseStartsAtTheFirstCountedDay(t *testing.T) {
	first := statsDate("2026-10-08")

	response := activityResponse([]db.RoomDailyStats{dailyRow("!room:x", "2026-10-08", 4, 12)}, &first, 30, statsToday)
	if response.From != "2026-10-08" || len(response.Daily) != 3 || response.Daily[0].Date != "2026-10-08" {
		t.Errorf("the period starts %s with %d days: %+v", response.From, len(response.Daily), response.Daily)
	}
	if response.Days != 30 {
		t.Errorf("the period asked for became %d days", response.Days)
	}
}

func TestActivityResponseWithNothingCounted(t *testing.T) {
	encoded, err := json.Marshal(activityResponse(nil, nil, 7, statsToday))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"days":7,"from":"2026-10-04","to":"2026-10-10","first_day":null,"messages":0,"words":0,"characters":0,"busiest":null,"daily":[]}`
	if string(encoded) != want {
		t.Errorf("nothing counted encodes as %s, want %s", encoded, want)
	}

	// Counted before, but nothing in the period
	first := statsDate("2026-02-06")
	response := activityResponse(nil, &first, 7, statsToday)
	if response.Busiest != nil || len(response.Daily) != 7 {
		t.Errorf("a quiet week has the busiest day %+v and %d days", response.Busiest, len(response.Daily))
	}
}

func TestPeopleResponseNamesThePeople(t *testing.T) {
	countedSince := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	lastSeen := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	totals := db.RoomMessageTotals{PeopleCount: 3, MessageCount: 200, WordCount: 1000, CharacterCount: 6000, CountedSince: &countedSince}
	bob := db.RankedMessageStats{MessageStats: db.MessageStats{UserID: "@bob:x", MessageCount: 120, WordCount: 300, LastSeen: lastSeen}, Rank: 1}
	carol := db.RankedMessageStats{MessageStats: db.MessageStats{UserID: "@carol:x", MessageCount: 50, WordCount: 500, LastSeen: lastSeen}, Rank: 2}
	alice := db.RankedMessageStats{MessageStats: db.MessageStats{UserID: "@alice:x", MessageCount: 30, WordCount: 200, LastSeen: lastSeen}, Rank: 3}
	bobName := "Bob"
	members := []db.RoomMember{{UserID: "@alice:x"}, {UserID: "@bob:x", DisplayName: &bobName}}

	response := peopleResponse(totals, []db.RankedMessageStats{bob, carol}, []db.RankedMessageStats{alice}, members)

	if response.PeopleCount != 3 || response.Messages != 200 || response.CountedSince == nil {
		t.Errorf("the totals are %+v", response)
	}
	want := []PersonStats{
		{UserID: "@bob:x", DisplayName: "Bob", InRoom: true, Rank: 1, Messages: 120, Words: 300, Share: 0.6, LastMessage: lastSeen},
		{UserID: "@carol:x", InRoom: false, Rank: 2, Messages: 50, Words: 500, Share: 0.25, LastMessage: lastSeen},
	}
	if len(response.Top) != len(want) {
		t.Fatalf("the top is %+v", response.Top)
	}
	for i := range want {
		if response.Top[i] != want[i] {
			t.Errorf("place %d is %+v, want %+v", i+1, response.Top[i], want[i])
		}
	}
	// A member without a display name is still in the room
	if response.Me == nil || !response.Me.InRoom || response.Me.DisplayName != "" || response.Me.Rank != 3 || response.Me.Share != 0.15 {
		t.Errorf("the viewer's own counts are %+v", response.Me)
	}

	// Someone with nothing counted has no counts of their own, and the list is empty rather than null
	encoded, err := json.Marshal(peopleResponse(db.RoomMessageTotals{}, nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"top":[]`) || !strings.Contains(string(encoded), `"me":null`) {
		t.Errorf("nothing counted encodes as %s", encoded)
	}
}

func TestRoomActivitiesRanksRoomsByMessages(t *testing.T) {
	rows := []db.RoomDailyStats{
		dailyRow("!quiet:x", "2026-10-09", 10, 40),
		dailyRow("!busy:x", "2026-10-09", 50, 100),
		dailyRow("!busy:x", "2026-10-10", 40, 80),
	}
	usage := []db.ChatUsageSummary{
		{RoomID: "!busy:x", Model: "model-a", Turns: 5},
		{RoomID: "!busy:x", Model: "model-b", Turns: 2},
		// A room the bot only chatted in, with no text messages counted
		{RoomID: "!images:x", Model: "model-a", Turns: 3},
	}
	names := map[string]string{"!busy:x": "Busy room"}

	rooms, chatTurns := roomActivities(rows, usage, func(roomID string) string { return names[roomID] })

	want := []RoomActivity{
		{RoomID: "!busy:x", RoomName: "Busy room", Messages: 90, Words: 180, Share: 0.9, ChatTurns: 7},
		{RoomID: "!quiet:x", Messages: 10, Words: 40, Share: 0.1},
		{RoomID: "!images:x", ChatTurns: 3},
	}
	if len(rooms) != len(want) {
		t.Fatalf("the rooms are %+v", rooms)
	}
	for i := range want {
		if rooms[i] != want[i] {
			t.Errorf("room %d is %+v, want %+v", i+1, rooms[i], want[i])
		}
	}
	if chatTurns != 10 {
		t.Errorf("%d chat turns in all, want 10", chatTurns)
	}
}

func TestStatsDays(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		query    string
		wantDays int
		wantOK   bool
	}{
		{"", defaultStatsDays, true},
		{"?days=7", 7, true},
		{"?days=90", 90, true},
		{"?days=0", 0, false},
		{"?days=91", 0, false},
		{"?days=a+week", 0, false},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/"+tt.query, nil)

		days, ok := statsDays(c, 90)
		if days != tt.wantDays || ok != tt.wantOK {
			t.Errorf("statsDays(%q) = %d, %v, want %d, %v", tt.query, days, ok, tt.wantDays, tt.wantOK)
		}
		if !ok && recorder.Code != http.StatusBadRequest {
			t.Errorf("statsDays(%q) responded %d, want 400", tt.query, recorder.Code)
		}
	}
}
