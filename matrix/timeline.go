package matrix

import (
	"sync"
	"time"

	"maunium.net/go/mautrix/event"
)

// An answer goes out as a plain message when nothing has been posted since the message it answers,
// the way people answer each other in a calm conversation, and as a reply to that message when the
// conversation has moved on. The decision is made just before the answer is sent (see Target), from
// the latest message of each timeline recorded here: a room's main timeline, or one of its threads.
//
// Only the latest message is kept, since that is all the decision asks about. Messages arriving
// through sync are recorded in timeline order, and the bot's own messages are recorded as soon as
// they are sent, so a decision made before their copy comes back through sync sees them too. When
// that copy arrives it is recorded again, which is right: sync delivers everything in timeline
// order, so whatever arrived before it came before it.

// timelineKey is where a message is posted: a room's main timeline, or a thread in it
type timelineKey struct {
	roomID       string
	threadRootID string
}

// timelineMemory is how long the latest message of a quiet timeline is remembered. It only has to
// outlast an answer's turn, including the turn's wait for the room. A message that is no longer
// remembered doesn't count as the latest one.
const timelineMemory = time.Hour

type latestMessage struct {
	eventID    string
	recordedAt time.Time
}

var timelines = struct {
	sync.Mutex
	latest map[timelineKey]latestMessage
}{latest: make(map[timelineKey]latestMessage)}

// recordLatest records a message as the latest one in its timeline
func recordLatest(key timelineKey, eventID string) {
	timelines.Lock()
	defer timelines.Unlock()

	now := time.Now()
	for k, latest := range timelines.latest {
		if now.Sub(latest.recordedAt) > timelineMemory {
			delete(timelines.latest, k)
		}
	}
	timelines.latest[key] = latestMessage{eventID: eventID, recordedAt: now}
}

// isLatest reports whether a message is still the latest one in its timeline
func isLatest(key timelineKey, eventID string) bool {
	timelines.Lock()
	defer timelines.Unlock()

	latest, ok := timelines.latest[key]
	return ok && latest.eventID == eventID && time.Since(latest.recordedAt) <= timelineMemory
}

// trackTimeline records a message arriving through sync as the latest one in its timeline. It is
// given every event, before anything else handles it, and picks out the ones that post a message.
//
// Encrypted messages are recorded as they arrive, whether or not they can be decrypted: where a
// message goes is in the clear, and each message is recorded once, whoever sent it. Edits, reactions
// and references such as poll votes attach to a message instead of posting one, so they don't count.
// Neither do state events, such as joins and renames.
func trackTimeline(evt *event.Event) {
	if evt.StateKey != nil || evt.Mautrix.EventSource&event.SourceTimeline == 0 {
		return
	}

	var rel *event.RelatesTo
	switch evt.Type {
	case event.EventMessage, event.EventSticker:
		rel = evt.Content.AsMessage().RelatesTo
	case event.EventEncrypted:
		rel = evt.Content.AsEncrypted().RelatesTo
	default:
		return
	}
	if rel.GetReplaceID() != "" || rel.GetAnnotationID() != "" || rel.GetReferenceID() != "" {
		return
	}

	recordLatest(timelineKey{roomID: evt.RoomID.String(), threadRootID: rel.GetThreadParent().String()}, evt.ID.String())
}
