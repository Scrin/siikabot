//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"maunium.net/go/mautrix/id"
)

// Images, uploaded to the homeserver as a client does and downloaded by the bot from its authenticated media
// endpoint

// testPNG is a small picture
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 12))
	for x := range 16 {
		for y := range 12 {
			img.Set(x, y, color.RGBA{R: uint8(x * 16), G: uint8(y * 20), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// SendImage uploads a picture and sends it, with caption as its caption if it isn't empty
func (u *User) SendImage(t *testing.T, room *Room, caption string, extra map[string]any) id.EventID {
	t.Helper()
	data := testPNG(t)
	uploaded, err := u.UploadBytes(context.Background(), data, "image/png")
	if err != nil {
		t.Fatalf("uploading a picture: %v", err)
	}
	content := map[string]any{
		"msgtype": "m.image", "body": "picture.png", "url": uploaded.ContentURI.String(),
		"info": map[string]any{"mimetype": "image/png", "w": 16, "h": 12, "size": len(data)},
	}
	if caption != "" {
		content["body"], content["filename"] = caption, "picture.png"
	}
	for k, v := range extra {
		content[k] = v
	}
	return u.Send(t, room, content)
}

// imagesShown is how many images the room's latest request showed the model
func imagesShown(t *testing.T, e *environment, room *Room) int {
	t.Helper()
	requests := e.Model.Requests(room.ID.String())
	if len(requests) == 0 {
		t.Fatal("the model was never asked")
	}
	return requests[len(requests)-1].ImageCount()
}

// An image whose caption addresses the bot is shown to the model, and kept with the turn
func TestImageWithACaption(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice := e.User(t, "alice")
	room := e.Room(t, alice, nil)

	asked := alice.SendImage(t, room, botName+": what is in this picture?", nil)
	room.WaitForBot(t, asked)
	if n := imagesShown(t, e, room); n != 1 {
		t.Fatalf("the model was shown %d images, want the one sent", n)
	}
	if stored := e.Query(t, "SELECT count(*) FROM chat_images WHERE event_id = $1", asked.String()); stored[0] != "1" {
		t.Errorf("%s images are stored for the message, want 1", stored[0])
	}

	// It's shown again while its turn is in the context window
	room.WaitForBot(t, alice.Ask(t, room, "and what colour is it?"))
	if n := imagesShown(t, e, room); n != 1 {
		t.Errorf("the next turn showed %d images, want the earlier one again", n)
	}
}

// An image the bot is asked about by a reply is shown; one nobody asked about isn't
func TestRepliedToImage(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})

	unasked := bob.SendImage(t, room, "", nil)
	picture := bob.SendImage(t, room, "", nil)
	question := e.Pill("what's this?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": picture}}
	room.WaitForBot(t, alice.Send(t, room, question))

	if n := imagesShown(t, e, room); n != 1 {
		t.Errorf("the model was shown %d images, want the replied-to one only", n)
	}
	if stored := e.Query(t, "SELECT count(*) FROM chat_images WHERE event_id = $1", unasked.String()); stored[0] != "0" {
		t.Error("an image nobody asked about was stored")
	}
}

// A redacted image leaves the history at once, whether it started the turn or was quoted in it
func TestRedactedImagesLeaveTheHistory(t *testing.T) {
	e := setUp(t)
	t.Parallel()
	alice, bob := e.User(t, "alice"), e.User(t, "bob")
	room := e.Room(t, alice, []*User{bob})

	own := alice.SendImage(t, room, botName+": look at this", nil)
	room.WaitForBot(t, own)
	quoted := bob.SendImage(t, room, "", nil)
	question := e.Pill("and this one?")
	question["m.relates_to"] = map[string]any{"m.in_reply_to": map[string]any{"event_id": quoted}}
	room.WaitForBot(t, alice.Send(t, room, question))
	if stored := e.Query(t, "SELECT count(*) FROM chat_images WHERE history_id IN (SELECT id FROM chat_history WHERE room_id = $1)",
		room.ID.String()); stored[0] != "2" {
		t.Fatalf("%s images are stored, want both", stored[0])
	}

	alice.Redact(t, room, own)
	bob.Redact(t, room, quoted)
	eventually(t, "the images to leave the history", func() bool {
		return e.Query(t, "SELECT count(*) FROM chat_images WHERE event_id IN ($1, $2)", own.String(), quoted.String())[0] == "0"
	})
	if turns := e.Query(t, "SELECT count(*) FROM chat_history WHERE turn_event_id = $1", own.String()); turns[0] != "0" {
		t.Errorf("the turn the redacted image started still has %s rows", turns[0])
	}

	room.WaitForBot(t, alice.Ask(t, room, "what did you see?"))
	if n := imagesShown(t, e, room); n != 0 {
		t.Errorf("after the redactions the model was shown %d images", n)
	}
}
