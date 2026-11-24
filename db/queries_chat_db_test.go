package db

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
)

// The chat history's SQL, run on a database of the test's own (see useTestDatabase)

const (
	roomA = "!a:example.com"
	roomB = "!b:example.com"
	alice = "@alice:example.com"
	bob   = "@bob:example.com"
	bot   = "@siikabot:example.com"
)

var chatT0 = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// question is a user turn with the given message, in a room's main timeline
func question(roomID, eventID, sender, message string) UserTurn {
	return UserTurn{
		Turn:       Turn{RoomID: roomID, EventID: eventID},
		UserID:     sender,
		SenderName: sender[1:6],
		SentAt:     chatT0,
		Message:    message,
	}
}

// inThread puts a turn in a thread
func inThread(turn UserTurn, rootID string) UserTurn {
	turn.ThreadRootID = rootID
	return turn
}

// storeTurn stores a whole turn: the question, a tool call with its result if tool isn't empty, and the
// answer, delivered as answerEventID
func storeTurn(t *testing.T, turn UserTurn, tool, answer, answerEventID string) {
	t.Helper()
	ctx := context.Background()
	if err := SaveUserTurn(ctx, turn); err != nil {
		t.Fatalf("SaveUserTurn() error = %v", err)
	}
	if tool != "" {
		records := []ToolCallRecord{{ToolCallID: "call-" + turn.EventID, ToolName: tool, Arguments: "{}", Response: tool + " result"}}
		if err := SaveToolCallsWithResponses(ctx, turn.Turn, turn.UserID, records); err != nil {
			t.Fatalf("SaveToolCallsWithResponses() error = %v", err)
		}
	}
	if err := SaveAnswer(ctx, turn.Turn, turn.UserID, answer, answerEventID); err != nil {
		t.Fatalf("SaveAnswer() error = %v", err)
	}
}

// history reads a timeline's history, with a seed turn if seed isn't empty, from its anchor if it has one
func history(t *testing.T, timeline Timeline, seed string) []ChatMessage {
	t.Helper()
	ctx := context.Background()
	anchor, err := GetChatContextAnchor(ctx, timeline)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := GetChatHistory(ctx, timeline, seed, anchor, 1000)
	if err != nil {
		t.Fatalf("GetChatHistory() error = %v", err)
	}
	return rows
}

// rowsOf describes rows for comparing: each one's type and text
func rowsOf(rows []ChatMessage) []string {
	described := make([]string, len(rows))
	for i, row := range rows {
		described[i] = row.MessageType + ":" + row.Message
	}
	return described
}

// userRow finds the user row of a turn
func userRow(t *testing.T, rows []ChatMessage, turnEventID string) ChatMessage {
	t.Helper()
	for _, row := range rows {
		if row.Role == "user" && row.TurnEventID == turnEventID {
			return row
		}
	}
	t.Fatalf("no user row of %s in %q", turnEventID, rowsOf(rows))
	return ChatMessage{}
}

func assertRows(t *testing.T, what string, got []ChatMessage, want ...string) {
	t.Helper()
	if fmt.Sprint(rowsOf(got)) != fmt.Sprint(want) {
		t.Errorf("%s = %q, want %q", what, rowsOf(got), want)
	}
}

// count counts the rows of a table
func count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A turn reads back as it was stored, with what it refers to in order, and the images it showed
func TestChatHistoryRoundTrip(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	turn := question(roomA, "$q1", alice, "what do you think of these?")
	turn.Mentions = []Mention{{UserID: bob, Name: "Bob"}}
	turn.UnseenBefore = 4
	turn.HasImage = true
	turn.ReplyTo = &QuotedMessage{EventID: "$r", Sender: bob, SenderName: "Bob", SentAt: chatT0.Add(-time.Hour),
		Kind: QuoteText, Body: "my plan"}
	turn.Links = []QuotedMessage{
		{EventID: "$l1", Sender: bob, SenderName: "Bob", SentAt: chatT0.Add(-2 * time.Hour), Kind: QuoteImage, Body: "a cat"},
		{EventID: "$l2", Sender: bot, SenderName: "Siikabot", SentAt: chatT0.Add(-3 * time.Hour), Kind: QuoteText,
			Body: "deploy done", RelayedBy: "deploys"},
	}
	turn.Images = []ChatImage{
		{EventID: "$q1", MIMEType: "image/png", Width: 10, Height: 20, Data: []byte("own image")},
		{EventID: "$l1", MIMEType: "image/jpeg", Width: 30, Height: 40, Data: []byte("linked image")},
	}
	if err := SaveUserTurn(ctx, turn); err != nil {
		t.Fatal(err)
	}
	records := []ToolCallRecord{
		{ToolCallID: "c1", ToolName: "get_time", Arguments: `{"tz":"Europe/Helsinki"}`, Response: "12:00"},
		{ToolCallID: "c2", ToolName: "get_fmi_data", Arguments: `{}`, Response: "-3 °C", ValidityDuration: time.Hour},
	}
	if err := SaveToolCallsWithResponses(ctx, turn.Turn, alice, records); err != nil {
		t.Fatal(err)
	}
	if err := SaveAnswer(ctx, turn.Turn, alice, "nice", "$a1"); err != nil {
		t.Fatal(err)
	}

	rows := history(t, Timeline{RoomID: roomA}, "")
	// The calls of a batch come before its results, in the order they were made, though every row of a
	// transaction has the same timestamp
	assertRows(t, "the history", rows, "text:what do you think of these?", `tool_call:{"tz":"Europe/Helsinki"}`,
		"tool_call:{}", "tool_response:12:00", "tool_response:-3 °C", "text:nice")
	if rows[2].Expiry == nil || rows[4].Expiry == nil || rows[1].Expiry != nil {
		t.Error("a tool call and its result don't share the call's expiry")
	}
	if rows[5].AnswerEventID == nil || *rows[5].AnswerEventID != "$a1" {
		t.Errorf("the answer's event is %v, want $a1", rows[5].AnswerEventID)
	}

	got := rows[0].UserTurn()
	if got.UserID != alice || got.UnseenBefore != 4 || !got.HasImage || len(got.Mentions) != 1 || got.Mentions[0].Name != "Bob" {
		t.Errorf("the user turn reads back as %+v", got)
	}
	if got.ReplyTo == nil || got.ReplyTo.Body != "my plan" || !got.ReplyTo.SentAt.Equal(turn.ReplyTo.SentAt) {
		t.Errorf("the reply reads back as %+v", got.ReplyTo)
	}
	if len(got.Links) != 2 || got.Links[0].EventID != "$l1" || got.Links[0].Kind != QuoteImage ||
		got.Links[0].Body != "a cat" || got.Links[1].RelayedBy != "deploys" {
		t.Errorf("the links read back as %+v", got.Links)
	}

	// The images come without their data until it's asked for
	if linked, ok := got.ImageOf("$l1"); len(got.Images) != 2 || !ok || linked.Data != nil || linked.Width != 30 || linked.MIMEType != "image/jpeg" {
		t.Fatalf("the images read back as %+v", got.Images)
	}
	if err := LoadChatImageData(ctx, roomA, rows); err != nil {
		t.Fatal(err)
	}
	if image, ok := rows[0].UserTurn().ImageOf("$l1"); !ok || !bytes.Equal(image.Data, []byte("linked image")) {
		t.Errorf("the linked image's data is %q", image.Data)
	}
}

// Each timeline has a context of its own, and a thread started from a stored turn begins with that turn
func TestChatHistoryKeepsTimelinesApart(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	storeTurn(t, question(roomA, "$q1", alice, "main question"), "", "main answer", "$a1")
	storeTurn(t, inThread(question(roomA, "$q2", bob, "thread question"), "$a1"), "get_time", "thread answer", "$a2")
	storeTurn(t, question(roomB, "$q3", alice, "other room"), "", "other answer", "$a3")

	assertRows(t, "the main timeline", history(t, Timeline{RoomID: roomA}, ""), "text:main question", "text:main answer")
	assertRows(t, "the thread with its seed", history(t, Timeline{RoomID: roomA, ThreadRootID: "$a1"}, "$q1"),
		"text:main question", "text:main answer", "text:thread question", "tool_call:{}", "tool_response:get_time result",
		"text:thread answer")
	assertRows(t, "the thread without a seed", history(t, Timeline{RoomID: roomA, ThreadRootID: "$a1"}, ""),
		"text:thread question", "tool_call:{}", "tool_response:get_time result", "text:thread answer")
	assertRows(t, "the other room", history(t, Timeline{RoomID: roomB}, "$q1"), "text:other room", "text:other answer")

	for _, test := range []struct {
		room, root string
		want       string
	}{
		{roomA, "$a1", "$q1"}, // a thread on the answer
		{roomA, "$q1", "$q1"}, // a thread on the question
		{roomA, "$q2", ""},    // a thread turn isn't a seed
		{roomA, "$a2", ""},
		{roomB, "$a1", ""}, // nor is a turn in another room
	} {
		seed, ok, err := GetThreadSeed(ctx, test.room, test.root)
		if err != nil {
			t.Fatal(err)
		}
		if ok != (test.want != "") || seed.TurnEventID != test.want {
			t.Errorf("GetThreadSeed(%s, %s) = %q, %v, want %q", test.room, test.root, seed.TurnEventID, ok, test.want)
		}
	}
}

// A turn reads its context window from the timeline's anchor on, with the newest rows kept under the cap
func TestChatHistoryReadsFromTheAnchor(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	timeline := Timeline{RoomID: roomA}

	for i := 1; i <= 3; i++ {
		storeTurn(t, question(roomA, fmt.Sprintf("$q%d", i), alice, fmt.Sprintf("q%d", i)), "", fmt.Sprintf("a%d", i),
			fmt.Sprintf("$a%d", i))
	}
	all := history(t, timeline, "")
	if err := SetChatContextAnchor(ctx, timeline, all[2].ID); err != nil {
		t.Fatal(err)
	}
	assertRows(t, "the history from the anchor", history(t, timeline, ""), "text:q2", "text:a2", "text:q3", "text:a3")

	anchor, _ := GetChatContextAnchor(ctx, timeline)
	capped, err := GetChatHistory(ctx, timeline, "", anchor, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, "the capped history", capped, "text:a2", "text:q3", "text:a3")

	if other, _ := GetChatContextAnchor(ctx, Timeline{RoomID: roomA, ThreadRootID: "$a1"}); other != nil {
		t.Errorf("a thread has the main timeline's anchor %d", *other)
	}
}

// A redacted message leaves the history: a turn it started or answered goes whole, and a quote of it is
// blanked, along with its image
func TestForgetChatEvent(t *testing.T) {
	setUp := func(t *testing.T) {
		useTestDatabase(t)
		storeTurn(t, question(roomA, "$q1", alice, "first"), "get_time", "first answer", "$a1")
		quoting := question(roomA, "$q2", bob, "what about this?")
		quoting.ReplyTo = &QuotedMessage{EventID: "$q1", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteText, Body: "first"}
		quoting.Links = []QuotedMessage{{EventID: "$img", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteImage, Body: "caption"}}
		quoting.HasImage = true
		quoting.Images = []ChatImage{
			{EventID: "$q2", MIMEType: "image/png", Width: 1, Height: 1, Data: []byte("own")},
			{EventID: "$img", MIMEType: "image/png", Width: 1, Height: 1, Data: []byte("linked")},
		}
		storeTurn(t, quoting, "", "second answer", "$a2")
		// The same event quoted in another room, which a redaction in the first room mustn't touch
		elsewhere := question(roomB, "$q3", alice, "elsewhere")
		elsewhere.Links = []QuotedMessage{{EventID: "$img", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteText, Body: "kept"}}
		storeTurn(t, elsewhere, "", "third answer", "$a3")
	}
	ctx := context.Background()

	// Redacting the question or the answer takes the whole turn. Only a quote of what was redacted is blanked:
	// with the answer redacted, the question is still there for everyone to see.
	for _, test := range []struct {
		redacted     string
		quoteBlanked bool
	}{{"$q1", true}, {"$a1", false}} {
		t.Run("turn_by_"+test.redacted, func(t *testing.T) {
			setUp(t)
			deleted, blanked, images, err := ForgetChatEvent(ctx, roomA, test.redacted)
			if err != nil {
				t.Fatal(err)
			}
			if wantBlanked := map[bool]int64{true: 1}[test.quoteBlanked]; deleted != 4 || blanked != wantBlanked || images != 0 {
				t.Errorf("ForgetChatEvent() = %d, %d, %d, want the turn's 4 rows and %d quotes", deleted, blanked, images, wantBlanked)
			}
			rows := history(t, Timeline{RoomID: roomA}, "")
			assertRows(t, "the history", rows, "text:what about this?", "text:second answer")
			if reply := rows[0].UserTurn().ReplyTo; (reply.Kind == QuoteDeleted && reply.Body == "") != test.quoteBlanked {
				t.Errorf("the quote of the question is %+v, want it blanked: %v", reply, test.quoteBlanked)
			}
		})
	}

	t.Run("quoted_image", func(t *testing.T) {
		setUp(t)
		deleted, blanked, images, err := ForgetChatEvent(ctx, roomA, "$img")
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 0 || blanked != 1 || images != 1 {
			t.Errorf("ForgetChatEvent() = %d, %d, %d, want the quote blanked and its image deleted", deleted, blanked, images)
		}
		rows := history(t, Timeline{RoomID: roomA}, "")
		turn := userRow(t, rows, "$q2").UserTurn()
		if turn.Links[0].Kind != QuoteDeleted || turn.Links[0].Body != "" || len(turn.Images) != 1 || turn.Images[0].EventID != "$q2" {
			t.Errorf("the turn quoting the image is %+v, with images %+v", turn.Links, turn.Images)
		}
		if again, _, _, _ := ForgetChatEvent(ctx, roomA, "$img"); again != 0 {
			t.Error("forgetting the event again deleted something")
		}
		if other := history(t, Timeline{RoomID: roomB}, "")[0].UserTurn(); other.Links[0].Body != "kept" {
			t.Error("a redaction in one room blanked a quote in another")
		}
	})
}

// A message its sender edits is stored as edited, in the turn it started and in quotes of it. Nobody else can
// rewrite it.
func TestFollowChatEdit(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	storeTurn(t, question(roomA, "$q1", alice, "my password is hunter2"), "", "noted", "$a1")
	quoting := question(roomA, "$q2", bob, "look")
	quoting.ReplyTo = &QuotedMessage{EventID: "$q1", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteText, Body: "my password is hunter2"}
	quoting.Links = []QuotedMessage{{EventID: "$img", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteImage, Body: "caption"}}
	storeTurn(t, quoting, "", "ok", "$a2")

	if turns, refs, err := FollowChatEdit(ctx, roomA, "$q1", "$e0", bob, "bob's version", "bob's version"); err != nil || turns != 0 || refs != 0 {
		t.Errorf("an edit by someone else changed %d turns and %d quotes, %v", turns, refs, err)
	}
	if turns, refs, err := FollowChatEdit(ctx, roomA, "$q1", "$e1", alice, "my password is ****", "my password is ****"); err != nil || turns != 1 || refs != 1 {
		t.Errorf("FollowChatEdit() = %d, %d, %v, want the turn and its quote", turns, refs, err)
	}
	// An image's caption can be edited away
	if _, refs, err := FollowChatEdit(ctx, roomA, "$img", "$e2", alice, "", ""); err != nil || refs != 1 {
		t.Errorf("removing a caption changed %d quotes, %v", refs, err)
	}

	rows := history(t, Timeline{RoomID: roomA}, "")
	assertRows(t, "the history", rows, "text:my password is ****", "text:noted", "text:look", "text:ok")
	turn := userRow(t, rows, "$q2").UserTurn()
	if turn.ReplyTo.Body != "my password is ****" || turn.Links[0].Body != "" || turn.Links[0].Kind != QuoteImage {
		t.Errorf("the quotes after the edits are %+v and %+v", turn.ReplyTo, turn.Links[0])
	}

	// A quote of a redacted message stays blank whatever happens to the message after
	if _, _, _, err := ForgetChatEvent(ctx, roomA, "$img"); err != nil {
		t.Fatal(err)
	}
	if _, refs, _ := FollowChatEdit(ctx, roomA, "$img", "$e3", alice, "new caption", "new caption"); refs != 0 {
		t.Error("an edit brought a redacted quote back")
	}
}

// The history says which edit its text comes from, so a redaction of the edit finds the messages it changed, and
// none once a later edit, or the message as sent, has replaced its text
func TestMessagesEditedBy(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	storeTurn(t, question(roomA, "$q1", alice, "first"), "", "noted", "$a1")
	quoting := question(roomA, "$q2", bob, "look")
	quoting.ReplyTo = &QuotedMessage{EventID: "$q1", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteText, Body: "first"}
	storeTurn(t, quoting, "", "ok", "$a2")
	storeTurn(t, question(roomB, "$q3", alice, "elsewhere"), "", "ok", "$a3")

	editedBy := func(room, edit string) []EditedMessage {
		t.Helper()
		messages, err := MessagesEditedBy(ctx, room, edit)
		if err != nil {
			t.Fatal(err)
		}
		return messages
	}
	if _, _, err := FollowChatEdit(ctx, roomA, "$q1", "$e1", alice, "second", "second"); err != nil {
		t.Fatal(err)
	}
	if got := editedBy(roomA, "$e1"); len(got) != 1 || got[0] != (EditedMessage{EventID: "$q1", Sender: alice}) {
		t.Errorf("the edit changed %+v, want the message once, its turn and its quote together", got)
	}
	if got := editedBy(roomB, "$e1"); len(got) != 0 {
		t.Errorf("the edit changed %+v in another room", got)
	}

	if _, _, err := FollowChatEdit(ctx, roomA, "$q1", "", alice, "first", "first"); err != nil {
		t.Fatal(err)
	}
	if got := editedBy(roomA, "$e1"); len(got) != 0 {
		t.Errorf("the edit still changed %+v after the message read as sent again", got)
	}

	// A quote of a message already edited when the turn quoted it holds that edit's text from the start
	late := question(roomA, "$q4", bob, "and this?")
	late.ReplyTo = &QuotedMessage{EventID: "$q9", Sender: alice, SenderName: "alice", SentAt: chatT0, Kind: QuoteText,
		Body: "edited before the quote", EditEventID: "$e9"}
	storeTurn(t, late, "", "ok", "$a4")
	if got := editedBy(roomA, "$e9"); len(got) != 1 || got[0] != (EditedMessage{EventID: "$q9", Sender: alice}) {
		t.Errorf("the edit a quote was stored with changed %+v, want the quoted message", got)
	}
}

// Resetting the main timeline leaves threads alone; resetting a thread drops the turn it was started from too
func TestResetChatTimeline(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	main, thread := Timeline{RoomID: roomA}, Timeline{RoomID: roomA, ThreadRootID: "$a1"}

	storeTurn(t, question(roomA, "$q1", alice, "main question"), "", "main answer", "$a1")
	storeTurn(t, inThread(question(roomA, "$q2", bob, "thread question"), "$a1"), "", "thread answer", "$a2")
	if err := SetChatContextAnchor(ctx, main, 1); err != nil {
		t.Fatal(err)
	}

	if n, err := ResetChatTimeline(ctx, thread); err != nil || n != 2 {
		t.Fatalf("resetting the thread deleted %d rows, %v, want its 2", n, err)
	}
	assertRows(t, "the thread with its seed, after its reset", history(t, thread, "$q1"))
	assertRows(t, "the main timeline after the thread's reset", history(t, main, ""), "text:main question", "text:main answer")
	storeTurn(t, inThread(question(roomA, "$q4", bob, "thread again"), "$a1"), "", "again", "$a4")
	assertRows(t, "the thread after a new turn", history(t, thread, "$q1"), "text:thread again", "text:again")

	if n, err := ResetChatTimeline(ctx, main); err != nil || n != 2 {
		t.Fatalf("resetting the main timeline deleted %d rows, %v, want its 2", n, err)
	}
	if anchor, _ := GetChatContextAnchor(ctx, main); anchor != nil {
		t.Errorf("the main timeline keeps its anchor %d after a reset", *anchor)
	}
	assertRows(t, "the thread after the main timeline's reset", history(t, thread, ""), "text:thread again", "text:again")
}

func TestHasChatTurn(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	storeTurn(t, question(roomA, "$q1", alice, "hi"), "", "hello", "$a1")

	for _, test := range []struct {
		room, event string
		want        bool
	}{{roomA, "$q1", true}, {roomA, "$a1", false}, {roomA, "$q2", false}, {roomB, "$q1", false}} {
		if got, err := HasChatTurn(ctx, test.room, test.event); err != nil || got != test.want {
			t.Errorf("HasChatTurn(%s, %s) = %v, %v, want %v", test.room, test.event, got, err, test.want)
		}
	}
}

func TestUnseenMessages(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	for range 3 {
		if err := CountUnseenMessage(ctx, roomA); err != nil {
			t.Fatal(err)
		}
	}
	_ = CountUnseenMessage(ctx, roomB)

	for _, test := range []struct {
		room string
		want int
	}{{roomA, 3}, {roomA, 0}, {roomB, 1}, {"!never:example.com", 0}} {
		if got, err := TakeUnseenMessages(ctx, test.room); err != nil || got != test.want {
			t.Errorf("TakeUnseenMessages(%s) = %d, %v, want %d", test.room, got, err, test.want)
		}
	}
}

// Old turns leave the history with what they quoted and showed
func TestCleanupOldChatHistory(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	old := question(roomA, "$q1", alice, "old")
	old.ReplyTo = &QuotedMessage{EventID: "$x", Sender: bob, SenderName: "bob", SentAt: chatT0, Kind: QuoteText, Body: "quoted"}
	old.Images = []ChatImage{{EventID: "$q1", MIMEType: "image/png", Width: 1, Height: 1, Data: []byte("x")}}
	storeTurn(t, old, "", "old answer", "$a1")
	storeTurn(t, question(roomA, "$q2", alice, "new"), "", "new answer", "$a2")
	if _, err := pool.Exec(ctx, "UPDATE chat_history SET timestamp = now() - interval '10 days' WHERE turn_event_id = '$q1'"); err != nil {
		t.Fatal(err)
	}

	if n, err := CleanupOldChatHistory(ctx, 7*24*time.Hour); err != nil || n != 2 {
		t.Errorf("CleanupOldChatHistory() = %d, %v, want the old turn's 2 rows", n, err)
	}
	assertRows(t, "the history", history(t, Timeline{RoomID: roomA}, ""), "text:new", "text:new answer")
	if count(t, "chat_references") != 0 || count(t, "chat_images") != 0 {
		t.Error("the old turn's quotes or images are still stored")
	}
}

// A pass over a room reads the people's messages and the answers in every timeline, never tool calls or results
func TestGetConversationsSince(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	storeTurn(t, question(roomA, "$q1", alice, "first"), "web_search", "first answer", "$a1")
	quoting := inThread(question(roomA, "$q2", bob, "in a thread"), "$a1")
	quoting.ReplyTo = &QuotedMessage{EventID: "$a1", Sender: bot, SenderName: "Siikabot", SentAt: chatT0, Kind: QuoteText, Body: "first answer"}
	storeTurn(t, quoting, "get_time", "thread answer", "$a2")
	storeTurn(t, question(roomB, "$q3", alice, "other room"), "", "other answer", "$a3")

	all := history(t, Timeline{RoomID: roomA}, "")
	thread := history(t, Timeline{RoomID: roomA, ThreadRootID: "$a1"}, "")
	after, until := all[0].ID, thread[len(thread)-1].ID
	rows, err := GetConversationsSince(ctx, roomA, after, until)
	if err != nil {
		t.Fatal(err)
	}
	assertRows(t, "the conversations after the first question", rows, "text:first answer", "text:in a thread", "text:thread answer")
	if reply := rows[1].UserTurn().ReplyTo; reply == nil || reply.Body != "first answer" {
		t.Errorf("the thread question's quote is %+v", reply)
	}
	if rows, _ := GetConversationsSince(ctx, roomA, 0, all[0].ID); len(rows) != 1 {
		t.Errorf("the range up to the first row read %d rows, want the first only", len(rows))
	}
}
