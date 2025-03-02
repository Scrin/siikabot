package matrix

import (
	"context"

	"github.com/Scrin/siikabot/db"
	mid "maunium.net/go/mautrix/id"
)

type SyncStore struct {
}

func NewSyncStore() *SyncStore {
	return &SyncStore{}
}

func (store *SyncStore) SaveFilterID(ctx context.Context, userID mid.UserID, filterID string) error {
	return db.SaveFilterID(ctx, userID, filterID)
}

func (store *SyncStore) LoadFilterID(ctx context.Context, userID mid.UserID) (string, error) {
	return db.LoadFilterID(ctx, userID)
}

func (store *SyncStore) SaveNextBatch(ctx context.Context, userID mid.UserID, nextBatchToken string) error {
	return db.SaveNextBatch(ctx, userID, nextBatchToken)
}

func (store *SyncStore) LoadNextBatch(ctx context.Context, userID mid.UserID) (string, error) {
	return db.LoadNextBatch(ctx, userID)
}
