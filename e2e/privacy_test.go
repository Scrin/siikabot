//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/testutil/fakenet"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// The chat privacy invariant (CLAUDE.md), checked against exactly what the model was shown. Each test has rooms of
// its own, so they run in parallel.

// marker is text no other message in any test has
func marker(t *testing.T, label string) string {
	return fmt.Sprintf("%s-%s-%s", label, strings.ReplaceAll(t.Name(), "/", "-"), randomString()[:8])
}

// conclude addresses the bot once more and waits for its answer, so that by then the bot has handled every message
// sent before, and checks of what the model saw are final
func conclude(t *testing.T, u *User, room *Room) {
	t.Helper()
	room.WaitForBot(t, u.Ask(t, room, "that's all"))
}

func assertSaw(t *testing.T, e *environment, room *Room, text string, want bool) {
	t.Helper()
	if saw := e.Model.Saw(room.ID.String(), text); saw != want {
		t.Errorf("the model saw %q: %v, want %v", text, saw, want)
	}
}

// What counts as addressing the bot (CLAUDE.md, the first bullet)
func TestWhatAddressesTheBot(t *testing.T) {
	e := setUp(t)

	for _, test := range []struct {
		name      string
		addressed bool
		send      func(t *testing.T, alice, bob *User, room *Room, text string)
	}{
		{"name_first", true, func(t *testing.T, alice, _ *User, room *Room, text string) {
			alice.Say(t, room, botName+", "+text)
		}},
		{"pill", true, func(t *testing.T, alice, _ *User, room *Room, text string) {
			alice.Send(t, room, e.Pill(text))
		}},
		{"reply_to_the_bot", true, func(t *testing.T, alice, _ *User, room *Room, text string) {
			answer := room.WaitForBot(t, alice.Ask(t, room, "hello"))
			alice.Reply(t, room, answer.ID, text)
		}},
		{"thread_on_the_bot", true, func(t *testing.T, alice, _ *User, room *Room, text string) {
			answer := room.WaitForBot(t, alice.Ask(t, room, "hello"))
			alice.Send(t, room, map[string]any{"msgtype": "m.text", "body": text, "m.relates_to": map[string]any{
				"rel_type": "m.thread", "event_id": answer.ID, "is_falling_back": true,
				"m.in_reply_to": map[string]any{"event_id": answer.ID},
			}})
		}},
		{"edit_that_mentions_the_bot", true, func(t *testing.T, alice, _ *User, room *Room, text string) {
			original := alice.Say(t, room, "a thought")
			pill := e.Pill(text)
			alice.Edit(t, room, original, pill["body"].(string), map[string]any{
				"format": pill["format"], "formatted_body": pill["formatted_body"], "m.mentions": pill["m.mentions"],
			})
		}},
		{"plain_message", false, func(t *testing.T, alice, _ *User, room *Room, text string) {
			alice.Say(t, room, text)
		}},
		{"name_later_on", false, func(t *testing.T, alice, _ *User, room *Room, text string) {
			alice.Say(t, room, "I told "+botName+" about "+text)
		}},
		{"reply_to_someone_else", false, func(t *testing.T, alice, bob *User, room *Room, text string) {
			alice.Reply(t, room, bob.Say(t, room, "bob's message"), text)
		}},
		{"edit_without_a_mention", false, func(t *testing.T, alice, _ *User, room *Room, text string) {
			alice.Edit(t, room, alice.Say(t, room, "a thought"), text, nil)
		}},
		{"thread_on_someone_else", false, func(t *testing.T, alice, bob *User, room *Room, text string) {
			root := bob.Say(t, room, "bob's message")
			alice.Send(t, room, map[string]any{"msgtype": "m.text", "body": text, "m.relates_to": map[string]any{
				"rel_type": "m.thread", "event_id": root, "is_falling_back": true,
				"m.in_reply_to": map[string]any{"event_id": root},
			}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			alice, bob := e.User(t, "alice"), e.User(t, "bob")
			room := e.Room(t, alice, []*User{bob})
			text := marker(t, "addressed")
			test.send(t, alice, bob, room, text)
			conclude(t, alice, room)

			assertSaw(t, e, room, text, test.addressed)
			stored := e.Query(t, "SELECT count(*) FROM chat_history WHERE room_id = $1 AND message LIKE '%' || $2 || '%'",
				room.ID.String(), text)
			if (stored[0] != "0") != test.addressed {
				t.Errorf("chat_history holds the message %s times, want it there: %v", stored[0], test.addressed)
			}
		})
	}
}

// The replied-to message and up to three linked ones reach the model, and nothing they refer to in turn
func TestReferencedMessages(t *testing.T) {
	e := setUp(t)

	t.Run("reply", func(t *testing.T) {
		t.Parallel()
		alice, bob := e.User(t, "alice"), e.User(t, "bob")
		room := e.Room(t, alice, []*User{bob})
		bobs := marker(t, "bobs")
		question := e.Pill("what do you think of this?")
		question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": bob.Say(t, room, bobs)}}
		room.WaitForBot(t, alice.Send(t, room, question))
		assertSaw(t, e, room, bobs, true)
	})

	t.Run("nested_reply_fallback", func(t *testing.T) {
		t.Parallel()
		alice, bob, carol := e.User(t, "alice"), e.User(t, "bob"), e.User(t, "carol")
		room := e.Room(t, alice, []*User{bob, carol})
		carols, bobs := marker(t, "carols"), marker(t, "bobs")
		carolsMessage := carol.Say(t, room, carols)
		// Bob's client quotes Carol's message in his reply to it
		bobsReply := bob.Send(t, room, map[string]any{
			"msgtype": "m.text", "body": fmt.Sprintf("> <%s> %s\n\n%s", carol.ID, carols, bobs),
			"format": "org.matrix.custom.html", "formatted_body": fmt.Sprintf(
				"<mx-reply><blockquote>%s</blockquote></mx-reply>%s", carols, bobs),
			"m.relates_to": map[string]any{"m.in_reply_to": map[string]any{"event_id": carolsMessage}},
		})
		question := e.Pill("and this?")
		question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": bobsReply}}
		room.WaitForBot(t, alice.Send(t, room, question))
		assertSaw(t, e, room, bobs, true)
		assertSaw(t, e, room, carols, false)
	})

	t.Run("links", func(t *testing.T) {
		t.Parallel()
		alice, bob := e.User(t, "alice"), e.User(t, "bob")
		room := e.Room(t, alice, []*User{bob})
		elsewhere := e.Room(t, bob, nil)
		var links []string
		var texts []string
		for i := range 4 {
			text := marker(t, fmt.Sprintf("linked%d", i))
			texts = append(texts, text)
			links = append(links, Link(room, bob.Say(t, room, text)))
		}
		otherRoom := marker(t, "other-room")
		links = append(links, Link(elsewhere, bob.Say(t, elsewhere, otherRoom)))
		room.WaitForBot(t, alice.Ask(t, room, "compare "+strings.Join(links, " ")))

		for i, text := range texts {
			assertSaw(t, e, room, text, i < 3)
		}
		assertSaw(t, e, room, otherRoom, false)
	})

	// A link may name the room by an alias, which counts only if it is an alias of the turn's room
	t.Run("links_through_aliases", func(t *testing.T) {
		t.Parallel()
		alice, bob := e.User(t, "alice"), e.User(t, "bob")
		room := e.Room(t, alice, []*User{bob})
		elsewhere := e.Room(t, bob, nil)
		server := e.botID.Homeserver()
		here, there := id.NewRoomAlias("here-"+randomString()[:8], server), id.NewRoomAlias("there-"+randomString()[:8], server)
		if _, err := alice.CreateAlias(context.Background(), here, room.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := bob.CreateAlias(context.Background(), there, elsewhere.ID); err != nil {
			t.Fatal(err)
		}
		inTheRoom, inAnother := marker(t, "aliased"), marker(t, "aliased-elsewhere")
		links := fmt.Sprintf("https://matrix.to/#/%s/%s https://matrix.to/#/%s/%s",
			here, bob.Say(t, room, inTheRoom), there, bob.Say(t, elsewhere, inAnother))
		room.WaitForBot(t, alice.Ask(t, room, "compare "+links))
		assertSaw(t, e, room, inTheRoom, true)
		assertSaw(t, e, room, inAnother, false)
	})

	t.Run("links_in_a_quote", func(t *testing.T) {
		t.Parallel()
		alice, bob := e.User(t, "alice"), e.User(t, "bob")
		room := e.Room(t, alice, []*User{bob})
		hidden := marker(t, "hidden")
		quoted := bob.Say(t, room, "see "+Link(room, bob.Say(t, room, hidden)))
		question := e.Pill("what's this about?")
		question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
		room.WaitForBot(t, alice.Send(t, room, question))
		assertSaw(t, e, room, hidden, false)
	})
}

// In a room that shows its history only from joining, someone can't get the bot to quote what was said before
// they joined
func TestReferencesRespectHistoryVisibility(t *testing.T) {
	e := setUp(t)
	for _, test := range []struct {
		visibility event.HistoryVisibility
		visible    bool
	}{{event.HistoryVisibilityJoined, false}, {event.HistoryVisibilityShared, true}} {
		t.Run(string(test.visibility), func(t *testing.T) {
			t.Parallel()
			alice, bob := e.User(t, "alice"), e.User(t, "bob")
			room := e.Room(t, alice, nil, WithHistoryVisibility(test.visibility))
			early := marker(t, "early")
			earlyMessage := alice.Say(t, room, early)
			room.Join(t, bob)
			room.WaitForBot(t, bob.Ask(t, room, "what does this mean? "+Link(room, earlyMessage)))
			assertSaw(t, e, room, early, test.visible)
		})
	}
}

// The tools that look up a room's members and the reminders take the room, and the person, from the turn: a
// conversation in one room never reaches another room's
func TestRoomScopedTools(t *testing.T) {
	e := setUp(t)

	// script calls a tool once, then answers
	script := func(t *testing.T, room *Room, tool, arguments string) {
		e.Model.Script(t, room.ID.String(), func(req fakenet.ModelRequest) fakenet.Reply {
			if len(req.TurnToolResults()) == 0 {
				return fakenet.CallTool("call-1", tool, arguments)
			}
			return fakenet.Answer("done")
		})
	}
	toolResult := func(t *testing.T, room *Room) string {
		t.Helper()
		for _, req := range e.Model.Requests(room.ID.String()) {
			if result, ok := req.ToolResults()["call-1"]; ok {
				return result
			}
		}
		t.Fatal("the model never got the tool's result")
		return ""
	}

	t.Run("reminders", func(t *testing.T) {
		t.Parallel()
		alice, bob := e.User(t, "alice"), e.User(t, "bob")
		room := e.Room(t, alice, []*User{bob})
		other := e.Room(t, alice, nil)
		here, there, bobs := marker(t, "here"), marker(t, "there"), marker(t, "bobs")
		room.WaitForBot(t, alice.Say(t, room, "!remind 1h "+here))
		other.WaitForBot(t, alice.Say(t, other, "!remind 1h "+there))
		room.WaitForBot(t, bob.Say(t, room, "!remind 1h "+bobs))

		script(t, room, "manage_reminders", `{"action":"list"}`)
		room.WaitForBot(t, alice.Ask(t, room, "what are my reminders?"))
		result := toolResult(t, room)
		if !strings.Contains(result, here) || strings.Contains(result, there) || strings.Contains(result, bobs) {
			t.Errorf("listing Alice's reminders in the room gave:\n%s\nwant only the one she set there", result)
		}
	})

	t.Run("members", func(t *testing.T) {
		t.Parallel()
		alice, bob, carol := e.User(t, "alice"), e.User(t, "bob"), e.User(t, "carol")
		room := e.Room(t, alice, []*User{bob})
		e.Room(t, carol, nil)

		script(t, room, "find_room_members", fmt.Sprintf(`{"query":%q}`, carol.ID.Localpart()))
		room.WaitForBot(t, alice.Ask(t, room, "is carol here?"))
		if result := toolResult(t, room); strings.Contains(result, carol.ID.String()) {
			t.Errorf("looking up Carol in a room she isn't in found her:\n%s", result)
		}
	})
}

// A message that never addressed the bot isn't in its history, so later turns can't bring it up either
func TestLaterTurnsDontSeeUnaddressedMessages(t *testing.T) {
	e := setUp(t)
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})

	first := marker(t, "first")
	room.WaitForBot(t, alice.Ask(t, room, first))
	between := marker(t, "between")
	bob.Say(t, room, between)
	room.WaitForBot(t, alice.Ask(t, room, "and now?"))

	requests := e.Model.Requests(room.ID.String())
	last := requests[len(requests)-1]
	if !strings.Contains(last.Text(), first) {
		t.Error("the second turn doesn't have the first in its history")
	}
	if strings.Contains(last.Text(), between) {
		t.Error("the second turn saw a message that never addressed the bot")
	}
}
