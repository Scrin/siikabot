//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/Scrin/siikabot/testutil/fakenet"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/attachment"
	"maunium.net/go/mautrix/event"
)

// End-to-end encrypted rooms, with test users that encrypt and decrypt as clients do

// As is the room as another member reads it
func (r *Room) As(u *User) *Room {
	return &Room{ID: r.ID, owner: u, env: r.env}
}

// assertNothingInTheClear checks that the homeserver holds no message of the room unencrypted
func assertNothingInTheClear(t *testing.T, u *User, room *Room) {
	t.Helper()
	resp, err := u.Messages(context.Background(), room.ID, "", "", mautrix.DirectionBackward, nil, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, evt := range resp.Chunk {
		if evt.Type == event.EventMessage {
			t.Errorf("a message from %s went out unencrypted", evt.Sender)
		}
	}
}

func TestEncryptedRoom(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice").Encrypted(t)
	room := e.Room(t, alice, nil, WithEncryption())
	question := marker(t, "question")

	if answer := room.WaitForBot(t, alice.Ask(t, room, question)); answer.Body != fakenet.DefaultAnswer {
		t.Errorf("in an encrypted room the bot answered %q", answer.Body)
	}
	assertSaw(t, e, room, question, true)
	assertNothingInTheClear(t, alice, room)
}

// An encrypted image is downloaded, decrypted and shown to the model
func TestEncryptedImage(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice").Encrypted(t)
	room := e.Room(t, alice, nil, WithEncryption())

	data := testPNG(t)
	file := attachment.NewEncryptedFile()
	file.EncryptInPlace(data)
	uploaded, err := alice.UploadBytes(context.Background(), data, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	asked := alice.Send(t, room, map[string]any{
		"msgtype": "m.image", "body": botName + ": what is in this picture?", "filename": "picture.png",
		"info": map[string]any{"mimetype": "image/png", "w": 16, "h": 12},
		"file": event.EncryptedFileInfo{EncryptedFile: *file, URL: uploaded.ContentURI.CUString()},
	})
	room.WaitForBot(t, asked)
	if n := imagesShown(t, e, room); n != 1 {
		t.Errorf("the model was shown %d images, want the encrypted one", n)
	}
	assertNothingInTheClear(t, alice, room)
}

// Someone who joins an encrypted room later gets the keys to read the bot's answers to them
func TestLateJoinerReadsTheBot(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice").Encrypted(t)
	bob := e.User(t, "bob").Encrypted(t)
	room := e.Room(t, alice, nil, WithEncryption())
	room.WaitForBot(t, alice.Ask(t, room, "hello"))

	room.Join(t, bob)
	asBob := room.As(bob)
	asBob.WaitUntilEncryptedFor(t, bob)
	if answer := asBob.WaitForBot(t, bob.Ask(t, asBob, "can you hear me?")); answer.Body != fakenet.DefaultAnswer {
		t.Errorf("Bob read the bot's answer as %q", answer.Body)
	}
}

// In an encrypted room the bot decrypts the message a reply quotes, and the one a link points to
func TestEncryptedReferences(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice").Encrypted(t), e.User(t, "bob").Encrypted(t)
	room := e.Room(t, alice, []*User{bob}, WithEncryption())
	asBob := room.As(bob)
	replied, linked := marker(t, "replied"), marker(t, "linked")

	question := e.Pill("what do you think of this?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": bob.Say(t, asBob, replied)}}
	room.WaitForBot(t, alice.Send(t, room, question))
	assertSaw(t, e, room, replied, true)

	room.WaitForBot(t, alice.Ask(t, room, "and "+Link(room, bob.Say(t, asBob, linked))))
	assertSaw(t, e, room, linked, true)
	assertNothingInTheClear(t, alice, room)
}
