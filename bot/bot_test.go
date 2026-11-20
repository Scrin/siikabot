package bot

import (
	"context"
	"encoding/json"
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

// Anyone in a room can send an event the bot doesn't expect. Events are handled in the sync loop,
// where an error ends the sync and the bot with it, and drops the rest of the batch, redactions
// included. However an event is handled, the sync has to carry on.
func TestUnexpectedEventsDontStopTheSync(t *testing.T) {
	for name, content := range map[string]string{
		"no_body":           `{"msgtype":"m.text"}`,
		"null_body":         `{"msgtype":"m.text","body":null}`,
		"number_body":       `{"msgtype":"m.text","body":42}`,
		"edit_without_body": `{"msgtype":"m.text","m.new_content":{"msgtype":"m.text"},"m.relates_to":{"rel_type":"m.replace","event_id":"$x"}}`,
		"image_no_caption":  `{"msgtype":"m.image","url":42}`,
	} {
		t.Run(name, func(t *testing.T) {
			syncer := mautrix.NewDefaultSyncer()
			syncer.OnEvent(func(ctx context.Context, evt *event.Event) { handleEvent(ctx, evt, false) })

			raw := `{"next_batch":"b","rooms":{"join":{"!r:example.com":{"timeline":{"events":[` +
				`{"type":"m.room.message","event_id":"$e","sender":"@mallory:example.com","origin_server_ts":1,` +
				`"content":` + content + `}]}}}}}`
			var res mautrix.RespSync
			if err := json.Unmarshal([]byte(raw), &res); err != nil {
				t.Fatal(err)
			}
			if err := syncer.ProcessResponse(context.Background(), &res, "a"); err != nil {
				t.Errorf("the sync stops on the event: %.300s", err)
			}
		})
	}
}
