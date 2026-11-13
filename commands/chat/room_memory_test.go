package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
)

// reflectionTime is when the conversations in these tests happened
var reflectionTime = time.Date(2026, 11, 13, 18, 30, 0, 0, time.UTC)

// questionRow is a stored message someone sent the bot, starting the turn eventID
func questionRow(eventID, message string) db.ChatMessage {
	return storedRow(db.UserTurn{
		Turn:       db.Turn{RoomID: "!room:example.com", EventID: eventID},
		UserID:     "@alice:example.com",
		SenderName: "Alice",
		SentAt:     reflectionTime,
		Message:    message,
	})
}

// answerRow is the bot's stored answer in the turn eventID
func answerRow(eventID, message string) db.ChatMessage {
	return db.ChatMessage{
		RoomID: "!room:example.com", UserID: "@alice:example.com", Message: message, Role: "assistant",
		MessageType: "text", TurnEventID: eventID, Timestamp: reflectionTime,
	}
}

// changesCall is a call of the tool a pass says what to change with
func changesCall(arguments string) []aigateway.ToolCall {
	return []aigateway.ToolCall{{ID: "call_1", Type: "function", Function: aigateway.ToolFunction{
		Name: reflectionTool.Function.Name, Arguments: arguments,
	}}}
}

// roomMemories are memories of a room with the given IDs
func roomMemories(ids ...int64) []db.RoomMemory {
	memories := make([]db.RoomMemory, len(ids))
	for i, id := range ids {
		memories[i] = db.RoomMemory{ID: id, RoomID: "!room:example.com", Memory: "memory"}
	}
	return memories
}

// A pass sees each question with its answer, in order. An answer whose question an earlier pass
// covered has nothing to go with, and a question the bot didn't answer is shown on its own.
func TestConversationsOf(t *testing.T) {
	conversations := conversationsOf([]db.ChatMessage{
		answerRow("$earlier", "an answer to a question already thought back over"),
		questionRow("$first", "is EVE down again?"),
		answerRow("$first", "Tranquility is in downtime until 11:15."),
		questionRow("$second", "thanks"),
	})

	if len(conversations) != 2 {
		t.Fatalf("got %d conversations, want 2: %#v", len(conversations), conversations)
	}
	if conversations[0].question.Message != "is EVE down again?" || conversations[0].answer != "Tranquility is in downtime until 11:15." {
		t.Errorf("the first conversation is %#v", conversations[0])
	}
	if conversations[1].question.Message != "thanks" || conversations[1].answer != "" {
		t.Errorf("the second conversation is %#v", conversations[1])
	}
}

// A conversation is shown as the chat showed it, with the header and the quotes, and without its
// images, which it says
func TestRenderConversation(t *testing.T) {
	row := questionRow("$q", "what is this ship?")
	hasImage := true
	row.HasImage = &hasImage
	row.ThreadRootID = ptr("$root")
	row.References = []db.Reference{{Relation: db.RelationReply, QuotedMessage: db.QuotedMessage{
		EventID: "$ship", Sender: "@bob:example.com", SenderName: "Bob", SentAt: reflectionTime, Kind: db.QuoteImage,
	}}}
	c := conversationsOf([]db.ChatMessage{row, answerRow("$q", "A Rifter.")})[0]

	got := renderConversation(3, c)
	for _, want := range []string{
		"### Conversation 3 (in a thread)\n",
		"[Alice (@alice:example.com) · ",
		"[replying to an image from Bob (@bob:example.com) · ",
		", not attached]",
		"[with an image, not attached]",
		"what is this ship?",
		"Your answer:\nA Rifter.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the conversation lacks %q:\n%s", want, got)
		}
	}

	unanswered := renderConversation(1, conversation{question: questionRow("$u", "hello").UserTurn()})
	if !strings.HasSuffix(unanswered, "(You didn't answer.)") {
		t.Errorf("an unanswered conversation reads:\n%s", unanswered)
	}
}

// The newest conversations that fit the budget are shown, and always at least the newest one
func TestReflectionInputKeepsTheNewest(t *testing.T) {
	var conversations []conversation
	for _, message := range []string{"alpha", "bravo", "charlie"} {
		conversations = append(conversations, conversation{
			question: questionRow("$"+message, message+" "+sizedText(1000)).UserTurn(),
			answer:   "ok",
		})
	}
	shows := func(text string) string {
		var shown []string
		for _, message := range []string{"alpha", "bravo", "charlie"} {
			if strings.Contains(text, message+" x") {
				shown = append(shown, message)
			}
		}
		return strings.Join(shown, " ")
	}

	all, shown := reflectionInput(conversations, 100000)
	if shown != 3 || shows(all) != "alpha bravo charlie" || !strings.HasPrefix(all, reflectionInputIntro) {
		t.Errorf("with room for everything, %d conversations are shown: %s", shown, shows(all))
	}
	if strings.Index(all, "alpha x") > strings.Index(all, "charlie x") {
		t.Error("the conversations aren't oldest first")
	}

	two, shown := reflectionInput(conversations, 2500)
	if shown != 2 || shows(two) != "bravo charlie" || !strings.Contains(two, "### Conversation 1\n[Alice") {
		t.Errorf("with room for two, %d conversations are shown: %s", shown, shows(two))
	}

	one, shown := reflectionInput(conversations, 10)
	if shown != 1 || shows(one) != "charlie" {
		t.Errorf("with room for none, %d conversations are shown, want the newest: %s", shown, shows(one))
	}
}

// The pass knows what it remembers now, by ID, so it can revise it
func TestReflectionPrompt(t *testing.T) {
	room := testRoom("EVE corp", alice, bob, botSelf)
	prompt := reflectionPrompt("Siikabot", room, []db.RoomMemory{
		{ID: 14, Memory: "People here ask for answers in Finnish."},
		{ID: 15, Memory: "Several people here play EVE Online."},
	})

	for _, want := range []string{
		"You are Siikabot (@siikabot:example.com)",
		`the group room "EVE corp"`,
		"Never keep a memory about one person",
		reflectionTool.Function.Name,
		"Keep at most 20",
		"## Your memories of this room now\n- [ID: 14] People here ask for answers in Finnish.\n- [ID: 15] Several people here play EVE Online.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %q:\n%s", want, prompt)
		}
	}

	if prompt := reflectionPrompt("Siikabot", testRoom("", alice, bob, botSelf), nil); !strings.HasSuffix(prompt, "None yet.") ||
		!strings.Contains(prompt, "in a group room,") {
		t.Errorf("the prompt of an unnamed room without memories:\n%s", prompt)
	}
}

// rejectedReasons lists why each rejected change was rejected, in order
func rejectedReasons(decisions []changeDecision) string {
	var reasons []string
	for _, d := range decisions {
		if !d.accepted() {
			reasons = append(reasons, d.reason)
		}
	}
	return strings.Join(reasons, " ")
}

func TestMemoryChanges(t *testing.T) {
	memories := []db.RoomMemory{
		{ID: 1, Memory: "first"},
		{ID: 2, Memory: "People here prefer Finnish."},
		{ID: 3, Memory: "Something that no longer holds."},
	}
	decisions, err := memoryChanges(changesCall(`{"changes": [
		{"action": "add", "memory_id": 2, "memory": "  Several people here play EVE Online.  "},
		{"action": "update", "memory_id": 2, "memory": "People here ask for answers in Finnish."},
		{"action": "delete", "memory_id": 3},
		{"action": "update", "memory_id": 3, "memory": "Too late: it was deleted above."},
		{"action": "delete", "memory_id": 99},
		{"action": "update", "memory": "No ID."},
		{"action": "add", "memory": ""},
		{"action": "add", "memory": "two\nlines"},
		{"action": "add", "memory": "`+strings.Repeat("x", maxRoomMemoryLength+1)+`"},
		{"action": "add", "memory": "`+strings.Repeat("ä", maxRoomMemoryLength)+`"},
		{"action": "forget"}
	]}`), memories)
	if err != nil {
		t.Fatalf("memoryChanges() failed: %v", err)
	}
	if len(decisions) != 11 {
		t.Fatalf("got %d decisions, want one for each of the 11 proposed changes", len(decisions))
	}

	want := []db.RoomMemoryChange{
		{Action: db.RoomMemoryAdd, Memory: "Several people here play EVE Online."},
		{Action: db.RoomMemoryUpdate, MemoryID: 2, Memory: "People here ask for answers in Finnish."},
		{Action: db.RoomMemoryDelete, MemoryID: 3},
		{Action: db.RoomMemoryAdd, Memory: strings.Repeat("ä", maxRoomMemoryLength)},
	}
	changes := acceptedChanges(decisions)
	if len(changes) != len(want) {
		t.Fatalf("accepted %d changes, want %d: %#v", len(changes), len(want), changes)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Errorf("change %d is %#v, want %#v", i, changes[i], want[i])
		}
	}

	// The memory's text before the change goes with an update or delete, for the logs and the trace
	if decisions[0].previous != "" || decisions[1].previous != "People here prefer Finnish." ||
		decisions[2].previous != "Something that no longer holds." {
		t.Errorf("the previous texts are %q, %q and %q", decisions[0].previous, decisions[1].previous, decisions[2].previous)
	}

	wantReasons := "unknown_memory unknown_memory unknown_memory empty multiple_lines too_long unknown_action"
	if got := rejectedReasons(decisions); got != wantReasons {
		t.Errorf("rejected for %q, want %q", got, wantReasons)
	}
}

// A full room takes a new memory only once a delete before it has freed a slot, so the pass can merge
// two memories into one
func TestMemoryChangesKeepToTheLimit(t *testing.T) {
	ids := make([]int64, maxRoomMemories)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	decisions, err := memoryChanges(changesCall(`{"changes": [
		{"action": "add", "memory": "No room for this."},
		{"action": "delete", "memory_id": 1},
		{"action": "add", "memory": "Room for this."},
		{"action": "add", "memory": "No room for this either."}
	]}`), roomMemories(ids...))
	if err != nil {
		t.Fatalf("memoryChanges() failed: %v", err)
	}
	changes := acceptedChanges(decisions)
	if len(changes) != 2 || changes[0].Action != db.RoomMemoryDelete || changes[1].Memory != "Room for this." {
		t.Errorf("accepted %#v", changes)
	}
	if got := rejectedReasons(decisions); got != "room_full room_full" {
		t.Errorf("rejected for %q", got)
	}
}

func TestMemoryChangesWithoutChanges(t *testing.T) {
	decisions, err := memoryChanges(nil, roomMemories(1))
	if err != nil || len(decisions) != 0 {
		t.Errorf("no tool call gives %#v, %v; want no change", decisions, err)
	}
}

// Arguments that can't be read fail the pass, which is then tried again
func TestMemoryChangesThatCantBeRead(t *testing.T) {
	if _, err := memoryChanges(changesCall(`{"changes": [{"action": "add", "memory": "cut off`), nil); err == nil {
		t.Error("cut-off arguments were read")
	}

	other := []aigateway.ToolCall{{ID: "call_1", Type: "function", Function: aigateway.ToolFunction{Name: "memory", Arguments: "{}"}}}
	if _, err := memoryChanges(other, nil); err == nil {
		t.Error("a call of another tool was read as changes")
	}
}

// A room is tried again after a failed pass, until the failures reach the limit and the bot moves on
// past the conversations that failed
func TestAfterFailure(t *testing.T) {
	outcome := reflectionRequestFailed
	base := db.RoomMemoryProgress{RoomID: "!room:example.com", ReflectedUntil: 10, LastOutcome: &outcome}
	for failedBefore := 0; failedBefore < maxReflectionFailures-1; failedBefore++ {
		progress, gaveUp := afterFailure(base, failedBefore, 20)
		if gaveUp || progress.FailedAttempts != failedBefore+1 || progress.ReflectedUntil != 10 {
			t.Fatalf("after %d failures: %#v, gave up: %v", failedBefore+1, progress, gaveUp)
		}
	}

	progress, gaveUp := afterFailure(base, maxReflectionFailures-1, 20)
	if !gaveUp || progress.FailedAttempts != 0 || progress.ReflectedUntil != 20 || *progress.LastOutcome != outcome {
		t.Errorf("after %d failures: %#v, gave up: %v", maxReflectionFailures, progress, gaveUp)
	}
}

func TestScheduleOf(t *testing.T) {
	now := time.Date(2026, 11, 13, 23, 0, 0, 0, time.UTC)
	delay := 30 * time.Minute
	at := func(minutesAgo int) *time.Time {
		t := now.Add(-time.Duration(minutesAgo) * time.Minute)
		return &t
	}

	for _, tt := range []struct {
		name  string
		state db.RoomMemoryState
		delay *time.Duration
		want  string
		// dueIn is how long from now the room is due, when it has new conversations and the passes are on
		dueIn time.Duration
	}{
		{"nothing new", db.RoomMemoryState{After: 10, Until: 10}, &delay, ScheduleUpToDate, 0},
		{"nothing new while off", db.RoomMemoryState{After: 10, Until: 10}, nil, ScheduleUpToDate, 0},
		{"new while off", db.RoomMemoryState{After: 10, Until: 12, LatestAt: at(60)}, nil, ScheduleOff, 0},
		{"still talking", db.RoomMemoryState{Until: 12, LatestAt: at(10)}, &delay, ScheduleWaiting, 20 * time.Minute},
		{"quiet long enough", db.RoomMemoryState{Until: 12, LatestAt: at(31)}, &delay, ScheduleDue, -time.Minute},
		{"quiet exactly long enough", db.RoomMemoryState{Until: 12, LatestAt: at(30)}, &delay, ScheduleDue, 0},
		{"quiet since a pass that went fine", db.RoomMemoryState{After: 5, Until: 12, LatestAt: at(40), LastAttemptAt: at(45)}, &delay, ScheduleDue, -10 * time.Minute},
		{"failed recently", db.RoomMemoryState{Until: 12, LatestAt: at(60), LastAttemptAt: at(5), FailedAttempts: 1}, &delay, ScheduleRetrying, 25 * time.Minute},
		{"failed long ago", db.RoomMemoryState{Until: 12, LatestAt: at(90), LastAttemptAt: at(35), FailedAttempts: 2}, &delay, ScheduleDue, -5 * time.Minute},
	} {
		got := ScheduleOf(tt.state, tt.delay, now)
		if got.Status != tt.want {
			t.Errorf("%s: status %s, want %s", tt.name, got.Status, tt.want)
			continue
		}
		hasDue := tt.want != ScheduleUpToDate && tt.want != ScheduleOff
		switch {
		case hasDue && got.DueAt == nil:
			t.Errorf("%s: no due time", tt.name)
		case hasDue && !got.DueAt.Equal(now.Add(tt.dueIn)):
			t.Errorf("%s: due at %s, want %s", tt.name, got.DueAt, now.Add(tt.dueIn))
		case !hasDue && got.DueAt != nil:
			t.Errorf("%s: due at %s, want no due time", tt.name, got.DueAt)
		}
	}
}

// A change whose memory the admin deleted while the pass ran is the one that didn't take effect
func TestNotApplied(t *testing.T) {
	changes := []db.RoomMemoryChange{
		{Action: db.RoomMemoryAdd, Memory: "new"},
		{Action: db.RoomMemoryUpdate, MemoryID: 2, Memory: "revised"},
		{Action: db.RoomMemoryDelete, MemoryID: 3},
		{Action: db.RoomMemoryAdd, Memory: "another"},
	}
	applied := []db.RoomMemoryChange{
		{Action: db.RoomMemoryAdd, MemoryID: 41, Memory: "new"},
		{Action: db.RoomMemoryDelete, MemoryID: 3},
		{Action: db.RoomMemoryAdd, MemoryID: 42, Memory: "another"},
	}

	missing := notApplied(changes, applied)
	if len(missing) != 1 || missing[0].Action != db.RoomMemoryUpdate || missing[0].MemoryID != 2 {
		t.Errorf("notApplied() = %#v, want the update of memory 2", missing)
	}
	if missing := notApplied(changes[:1], applied[:1]); len(missing) != 0 {
		t.Errorf("with everything applied, notApplied() = %#v", missing)
	}
}

// The turns a pass covers are named by their event IDs, which is all the logs say of them
func TestTurnEventIDs(t *testing.T) {
	conversations := conversationsOf([]db.ChatMessage{questionRow("$first", "a"), questionRow("$second", "b")})
	if got := strings.Join(turnEventIDs(conversations), " "); got != "$first $second" {
		t.Errorf("turnEventIDs() = %q", got)
	}
}
