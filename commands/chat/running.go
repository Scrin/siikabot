package chat

import (
	"context"
	"sync"

	"github.com/Scrin/siikabot/db"
)

// turnsStarted are the messages whose chat turn has started and isn't over, from when the message is
// routed to the bot until the turn ends. A turn stores its question only once the model has first
// answered, so until then only this says the message has one.
var turnsStarted sync.Map

func turnKey(roomID, eventID string) string {
	return roomID + "\x00" + eventID
}

// StartingTurn records that a message's chat turn is starting, until the returned function says it's
// over. Routing calls it as the message arrives, before the turn's own goroutine starts, so that an
// edit routed after the message finds the turn.
func StartingTurn(roomID, eventID string) (over func()) {
	key := turnKey(roomID, eventID)
	turnsStarted.Store(key, struct{}{})
	return func() { turnsStarted.Delete(key) }
}

// HasTurn reports whether a message has had a chat turn, or has one starting or running. An edit that
// newly mentions the bot starts a turn only for a message without one.
func HasTurn(ctx context.Context, roomID, eventID string) (bool, error) {
	if _, started := turnsStarted.Load(turnKey(roomID, eventID)); started {
		return true, nil
	}
	return db.HasChatTurn(ctx, roomID, eventID)
}
