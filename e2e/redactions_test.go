//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Scrin/siikabot/testutil/fakenet"
	"github.com/Scrin/siikabot/testutil/knownbug"
)

// Redacted messages leave the chat history, and edited ones are stored as edited (the chat privacy invariant's
// last bullets), including when the redaction or edit races the turn

// knownBug skips a test for a bug the review found and nothing has fixed yet (see testutil/knownbug)
func knownBug(t *testing.T, finding, what string) {
	t.Helper()
	knownbug.Skip(t, finding, what)
}

// historyHolds reports whether a room's chat history, its quotes included, holds text
func historyHolds(t *testing.T, e *environment, room *Room, text string) bool {
	t.Helper()
	rows := e.Query(t, `SELECT count(*) FROM chat_history h LEFT JOIN chat_references r ON r.history_id = h.id
		WHERE h.room_id = $1 AND (h.message LIKE '%' || $2 || '%' OR r.body LIKE '%' || $2 || '%')`, room.ID.String(), text)
	return rows[0] != "0"
}

// waitForTheTurn waits until a room's turn is waiting for the model: it has been routed, and what it refers to
// fetched, but nothing of it stored yet
func waitForTheTurn(t *testing.T, e *environment, room *Room) {
	t.Helper()
	eventually(t, "the turn to reach the model", func() bool { return len(e.Model.Requests(room.ID.String())) > 0 })
}

// heldModel holds the model's first answer in a room until the returned function releases it, so that a test can
// do things while the turn waits for the model, however long the bot takes to see them
func heldModel(t *testing.T, e *environment, room *Room) (release func()) {
	hold := make(chan struct{})
	first := true
	e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
		if first {
			first = false
			return fakenet.Reply{Text: fakenet.DefaultAnswer, Hold: hold}
		}
		return fakenet.Answer(fakenet.DefaultAnswer)
	})
	var once sync.Once
	release = func() { once.Do(func() { close(hold) }) }
	t.Cleanup(release)
	return release
}

// settle waits until the bot has handled everything sent to a room so far. The bot handles a room's events in
// order, and counts a message that doesn't address it as soon as it handles it, so once a probe is counted, what
// came before it has been handled.
func settle(t *testing.T, e *environment, room *Room, u *User) {
	t.Helper()
	before := e.Query(t, "SELECT coalesce(sum(count), 0) FROM chat_unseen_messages WHERE room_id = $1", room.ID.String())[0]
	u.Say(t, room, "probe")
	eventually(t, "the bot to catch up", func() bool {
		return e.Query(t, "SELECT coalesce(sum(count), 0) FROM chat_unseen_messages WHERE room_id = $1", room.ID.String())[0] != before
	})
}

func TestRedactedQuestionLeavesTheHistory(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	secret := marker(t, "secret")

	asked := alice.Ask(t, room, secret)
	room.WaitForBot(t, asked)
	alice.Redact(t, room, asked)
	eventually(t, "the question to leave the history", func() bool { return !historyHolds(t, e, room, secret) })

	room.WaitForBot(t, alice.Ask(t, room, "what did I say?"))
	requests := e.Model.Requests(room.ID.String())
	if last := requests[len(requests)-1]; strings.Contains(last.Text(), secret) {
		t.Error("the next turn still saw the redacted question")
	}
}

func TestRedactedQuoteIsBlanked(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	bobs := marker(t, "bobs")

	quoted := bob.Say(t, room, bobs)
	question := e.Pill("what do you think?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	room.WaitForBot(t, alice.Send(t, room, question))
	bob.Redact(t, room, quoted)
	eventually(t, "the quote to be blanked", func() bool { return !historyHolds(t, e, room, bobs) })
}

// Editing a question that addressed the bot changes the stored question
func TestEditedQuestionIsStoredAsEdited(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	before, after := marker(t, "before"), marker(t, "after")

	asked := alice.Ask(t, room, before)
	room.WaitForBot(t, asked)
	alice.Edit(t, room, asked, botName+": "+after, nil)
	eventually(t, "the edit to reach the history", func() bool {
		return historyHolds(t, e, room, after) && !historyHolds(t, e, room, before)
	})
}

// A quoted message redacted while the turn quoting it waits for the model (P1)
func TestQuoteRedactedDuringItsTurn(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	release := heldModel(t, e, room)
	bobs := marker(t, "bobs")

	quoted := bob.Say(t, room, bobs)
	question := e.Pill("what do you think?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	asked := alice.Send(t, room, question)
	waitForTheTurn(t, e, room)
	bob.Redact(t, room, quoted)
	settle(t, e, room, bob)
	release()
	room.WaitForBot(t, asked)

	if historyHolds(t, e, room, bobs) {
		t.Error("the redacted quote is stored")
	}
}

// A quoted message redacted while the turn quoting it waits for the room is never shown to the model, nor stored
// (P1). The turn fetched it when the message was routed, before the redaction.
func TestQuoteRedactedBeforeItsTurnStarts(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	release := heldModel(t, e, room)
	bobs := marker(t, "bobs")

	// The first turn keeps the room while the quoting one waits for it
	alice.Ask(t, room, "first question")
	waitForTheTurn(t, e, room)
	quoted := bob.Say(t, room, bobs)
	question := e.Pill("what do you think?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	alice.Send(t, room, question)
	settle(t, e, room, bob)
	bob.Redact(t, room, quoted)
	settle(t, e, room, bob)
	release()
	conclude(t, alice, room)

	assertSaw(t, e, room, bobs, false)
	if historyHolds(t, e, room, bobs) {
		t.Error("the redacted quote is stored")
	}
}

// A question edited while its own turn waits for the model (P2)
func TestQuestionEditedDuringItsTurn(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	release := heldModel(t, e, room)
	before, after := marker(t, "before"), marker(t, "after")

	asked := alice.Ask(t, room, before)
	waitForTheTurn(t, e, room)
	alice.Edit(t, room, asked, botName+": "+after, nil)
	settle(t, e, room, alice)
	release()
	room.WaitForBot(t, asked)

	if historyHolds(t, e, room, before) {
		t.Error("the question is stored as it was before the edit")
	}
}

// A question and the message it quotes, both edited while the turn waits for the room, reach the model and the
// history as edited (P2). The turn fetched the quote, and took the question, when the message was routed.
func TestEditsBeforeTheTurnStarts(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	release := heldModel(t, e, room)
	askedBefore, askedAfter := marker(t, "asked-before"), marker(t, "asked-after")
	quotedBefore, quotedAfter := marker(t, "quoted-before"), marker(t, "quoted-after")

	// The first turn keeps the room while the edited one waits for it
	alice.Ask(t, room, "first question")
	waitForTheTurn(t, e, room)
	quoted := bob.Say(t, room, quotedBefore)
	question := e.Pill(askedBefore)
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	asked := alice.Send(t, room, question)
	settle(t, e, room, bob)
	bob.Edit(t, room, quoted, quotedAfter, nil)
	alice.Edit(t, room, asked, botName+": "+askedAfter, nil)
	settle(t, e, room, alice)
	release()
	conclude(t, alice, room)

	for _, text := range []string{askedAfter, quotedAfter} {
		assertSaw(t, e, room, text, true)
		if !historyHolds(t, e, room, text) {
			t.Errorf("the history doesn't hold %q", text)
		}
	}
	for _, text := range []string{askedBefore, quotedBefore} {
		assertSaw(t, e, room, text, false)
		if historyHolds(t, e, room, text) {
			t.Errorf("the history holds %q, from before the edit", text)
		}
	}
}

// A quoted message edited while the turn quoting it waits for the model is stored as edited (P2)
func TestQuoteEditedDuringItsTurn(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	release := heldModel(t, e, room)
	before, after := marker(t, "before"), marker(t, "after")

	quoted := bob.Say(t, room, before)
	question := e.Pill("what do you think?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	asked := alice.Send(t, room, question)
	waitForTheTurn(t, e, room)
	bob.Edit(t, room, quoted, after, nil)
	settle(t, e, room, bob)
	release()
	room.WaitForBot(t, asked)

	if historyHolds(t, e, room, before) || !historyHolds(t, e, room, after) {
		t.Error("the quote is stored as it was before the edit")
	}
}

// An edit that newly pills the bot, of a message whose turn is still running, starts no second turn (P2). Until
// the turn stores its question, nothing in the history says the message has one.
func TestEditAddingAPillDuringItsTurn(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	release := heldModel(t, e, room)
	question := marker(t, "question")

	asked := alice.Ask(t, room, question)
	waitForTheTurn(t, e, room)
	pill := e.Pill(question + " again")
	alice.Edit(t, room, asked, pill["body"].(string), map[string]any{
		"format": pill["format"], "formatted_body": pill["formatted_body"], "m.mentions": pill["m.mentions"]})
	settle(t, e, room, alice)
	release()
	conclude(t, alice, room)

	turns := 0
	for _, req := range e.Model.Requests(room.ID.String()) {
		if strings.Contains(req.LastUserText(), question) {
			turns++
		}
	}
	if turns != 1 {
		t.Errorf("the message got %d turns, want 1", turns)
	}
}

// Redacting an edit takes its text out of the history (P3)
func TestRedactedEditLeavesTheHistory(t *testing.T) {
	knownBug(t, "P3", "redacting an edit leaves the edit's text stored")
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	edited := marker(t, "edited")

	asked := alice.Ask(t, room, "first version")
	room.WaitForBot(t, asked)
	edit := alice.Edit(t, room, asked, botName+": "+edited, nil)
	eventually(t, "the edit to reach the history", func() bool { return historyHolds(t, e, room, edited) })
	alice.Redact(t, room, edit)

	time.Sleep(2 * time.Second)
	if historyHolds(t, e, room, edited) {
		t.Error("the redacted edit's text is still stored")
	}
}

// A quoted message is quoted as edited (P4)
func TestQuotedMessageIsQuotedAsEdited(t *testing.T) {
	knownBug(t, "P4", "replied-to and linked messages are read as originally sent")
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})
	before, after := marker(t, "before"), marker(t, "after")

	quoted := bob.Say(t, room, before)
	bob.Edit(t, room, quoted, after, nil)
	question := e.Pill("what do you think?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	room.WaitForBot(t, alice.Send(t, room, question))

	assertSaw(t, e, room, after, true)
	assertSaw(t, e, room, before, false)
}

// A redaction made while the bot is down still takes the message out of the history when the bot starts again: when
// the redaction is in the sync the bot catches up with, and when more events than that came after it, which leaves
// it in the gap behind the sync (P6). It stops the bot, so it doesn't run in parallel.
func TestRedactionWhileTheBotIsDown(t *testing.T) {
	e := setUp(t)
	for _, test := range []struct {
		name    string
		fillers int
		bug     bool
	}{{"in_the_sync", 5, false}, {"behind_a_sync_gap", 60, true}} {
		t.Run(test.name, func(t *testing.T) {
			if test.bug {
				knownBug(t, "P6", "redactions and edits that fall in a sync gap are never applied")
			}
			alice := e.User(t, "alice")
			room := e.Room(t, alice, nil)
			secret := marker(t, "secret")

			asked := alice.Ask(t, room, secret)
			room.WaitForBot(t, asked)
			e.StopBot(t)
			alice.Redact(t, room, asked)
			for i := range test.fillers {
				alice.Say(t, room, fmt.Sprintf("filler %d", i))
			}
			e.StartBot(t)
			conclude(t, alice, room)

			if historyHolds(t, e, room, secret) {
				t.Error("a question redacted while the bot was down is still in its history")
			}
		})
	}
}

// A reminder set before a restart still goes off after it
func TestReminderSurvivesARestart(t *testing.T) {
	e := setUp(t)
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)
	text := marker(t, "survives")

	set := alice.Say(t, room, "!remind 8s "+text)
	room.WaitForBot(t, set)
	e.StopBot(t)
	e.StartBot(t)
	room.WaitForBotMessage(t, set, func(content map[string]any) bool {
		body, _ := content["body"].(string)
		return strings.Contains(body, text) && !strings.Contains(body, "Reminding")
	})
}
