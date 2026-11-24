package matrix

import (
	"context"
	"slices"

	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// A sync after a break, such as the bot being down, gives each room at most the latest events of its
// timeline, and says the timeline is limited when it left some out. The redactions and edits among
// those still matter, as the chat history follows them, so the gap behind a limited timeline is read
// back to where the previous sync ended, and they are handed on. Nothing else in the gap is, as
// nothing in it was seen when it happened.

// A gap is read a page at a time, and no more than maxGapPages of it: a room busy for days while the
// bot was away has more behind it than the chat history keeps
const (
	gapPageSize = 100
	maxGapPages = 50
)

// missedChangesHandler is a sync listener that hands on the redactions and edits in the gaps behind a
// sync's limited timelines, oldest first, before the sync's own events are handled
func missedChangesHandler(handle func(ctx context.Context, evt *event.Event)) mautrix.SyncHandler {
	return func(ctx context.Context, resp *mautrix.RespSync, since string) bool {
		if since == "" {
			// The first sync has nothing before it to miss
			return true
		}
		for roomID, room := range resp.Rooms.Join {
			if !room.Timeline.Limited || room.Timeline.PrevBatch == "" {
				continue
			}
			for _, evt := range missedChanges(ctx, roomID, room.Timeline.PrevBatch, since) {
				handle(ctx, evt)
			}
		}
		return true
	}
}

// missedChanges reads a room's timeline back from the start of a limited timeline to where the
// previous sync ended, and returns the redactions and edits in it, oldest first, decrypted if they
// were encrypted. What a failure leaves unread is logged.
func missedChanges(ctx context.Context, roomID id.RoomID, from, to string) []*event.Event {
	filter := &mautrix.FilterPart{Types: []event.Type{event.EventRedaction, event.EventMessage, event.EventEncrypted}}
	var changes []*event.Event
	read := 0
	for page := 0; ; page++ {
		if page == maxGapPages {
			log.Warn().Ctx(ctx).
				Str("room_id", roomID.String()).
				Int("events_read", read).
				Msg("Leaving the rest of a gap in a room's timeline unread")
			break
		}
		resp, err := client.Messages(ctx, roomID, from, to, mautrix.DirectionBackward, filter, gapPageSize)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID.String()).
				Int("events_read", read).
				Msg("Failed to read a gap in a room's timeline")
			break
		}
		for _, evt := range resp.Chunk {
			evt.RoomID = roomID
			evt = decrypted(ctx, roomID.String(), evt)
			// A decrypted event is parsed already
			_ = evt.Content.ParseRaw(evt.Type)
			isEdit := evt.Type == event.EventMessage && evt.Content.AsMessage().RelatesTo.GetReplaceID() != ""
			if evt.Type == event.EventRedaction || isEdit {
				changes = append(changes, evt)
			}
		}
		read += len(resp.Chunk)
		if len(resp.Chunk) == 0 || resp.End == "" || resp.End == from {
			break
		}
		from = resp.End
	}
	slices.Reverse(changes)

	log.Debug().Ctx(ctx).
		Str("room_id", roomID.String()).
		Int("events_read", read).
		Int("changes", len(changes)).
		Msg("Read a gap in a room's timeline")
	return changes
}
