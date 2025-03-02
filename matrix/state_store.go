package matrix

import (
	"context"

	"github.com/Scrin/siikabot/db"
	"maunium.net/go/mautrix/event"
	mid "maunium.net/go/mautrix/id"
)

type StateStore struct {
}

func NewStateStore() *StateStore {
	return &StateStore{}
}

func (store *StateStore) IsEncrypted(ctx context.Context, roomID mid.RoomID) (bool, error) {
	encryptionEvent, err := store.GetEncryptionEvent(ctx, roomID)
	if err != nil {
		return false, err
	}
	return encryptionEvent != nil, nil
}

func (store *StateStore) GetEncryptionEvent(ctx context.Context, roomId mid.RoomID) (*event.EncryptionEventContent, error) {
	return db.GetEncryptionEvent(ctx, roomId)
}

func (store *StateStore) SetEncryptionEvent(ctx context.Context, event *event.Event) error {
	return db.SaveEncryptionEvent(ctx, event.RoomID, &event.Content)
}

func (store *StateStore) FindSharedRooms(ctx context.Context, userId mid.UserID) ([]mid.RoomID, error) {
	return db.FindSharedRooms(ctx, userId)
}

func (store *StateStore) SetMembership(ctx context.Context, event *event.Event) error {
	return db.SaveRoomMember(ctx, event.RoomID, event.Sender)
}

func (store *StateStore) GetRoomMembers(ctx context.Context, roomId mid.RoomID) ([]mid.UserID, error) {
	return db.GetRoomMembers(ctx, roomId)
}
