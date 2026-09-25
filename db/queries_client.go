package db

import (
	"context"
	"encoding/json"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
	mid "maunium.net/go/mautrix/id"
)

// SaveFilterID saves or updates a filter ID for a user
func SaveFilterID(ctx context.Context, userID mid.UserID, filterID string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO user_filter_ids (user_id, filter_id) 
		 VALUES ($1, $2) 
		 ON CONFLICT (user_id) 
		 DO UPDATE SET filter_id = $2`,
		userID, filterID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", string(userID)).
			Str("filter_id", filterID).
			Msg("Failed to save filter ID")
		return err
	}
	return nil
}

// LoadFilterID loads a filter ID for a user
func LoadFilterID(ctx context.Context, userID mid.UserID) (string, error) {
	var filterID string
	err := pool.QueryRow(ctx,
		"SELECT filter_id FROM user_filter_ids WHERE user_id = $1",
		userID).Scan(&filterID)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Error().Ctx(ctx).Err(err).
				Str("user_id", string(userID)).
				Msg("Failed to load filter ID")
			return "", err
		}
		return "", nil
	}
	return filterID, nil
}

// SaveNextBatch saves or updates a next batch token for a user
func SaveNextBatch(ctx context.Context, userID mid.UserID, nextBatchToken string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO user_batch_tokens (user_id, next_batch_token) 
		 VALUES ($1, $2) 
		 ON CONFLICT (user_id) 
		 DO UPDATE SET next_batch_token = $2`,
		userID, nextBatchToken)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", string(userID)).
			Str("next_batch_token", nextBatchToken).
			Msg("Failed to save next batch token")
		return err
	}
	return nil
}

// LoadNextBatch loads a next batch token for a user
func LoadNextBatch(ctx context.Context, userID mid.UserID) (string, error) {
	var nextBatchToken string
	err := pool.QueryRow(ctx,
		"SELECT next_batch_token FROM user_batch_tokens WHERE user_id = $1",
		userID).Scan(&nextBatchToken)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Error().Ctx(ctx).Err(err).
				Str("user_id", string(userID)).
				Msg("Failed to load next batch token")
			return "", err
		}
		return "", nil
	}
	return nextBatchToken, nil
}

// SaveEncryptionEvent saves or updates an encryption event for a room
func SaveEncryptionEvent(ctx context.Context, roomID mid.RoomID, encryptionEvent *event.Content) error {
	eventJSON, err := json.Marshal(encryptionEvent)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", string(roomID)).
			Msg("Failed to marshal encryption event")
		return err
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO rooms (room_id, encryption_event) 
		 VALUES ($1, $2) 
		 ON CONFLICT (room_id) 
		 DO UPDATE SET encryption_event = $2`,
		roomID, eventJSON)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", string(roomID)).
			Msg("Failed to save encryption event")
		return err
	}
	return nil
}

// GetEncryptionEvent gets an encryption event for a room
func GetEncryptionEvent(ctx context.Context, roomID mid.RoomID) (*event.EncryptionEventContent, error) {
	var encryptionEventJSON []byte
	err := pool.QueryRow(ctx,
		"SELECT encryption_event FROM rooms WHERE room_id = $1",
		roomID).Scan(&encryptionEventJSON)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", string(roomID)).
				Msg("Failed to get encryption event")
		}
		return nil, err
	}

	if encryptionEventJSON == nil {
		return nil, nil
	}

	var encryptionEvent event.EncryptionEventContent
	if err := json.Unmarshal(encryptionEventJSON, &encryptionEvent); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", string(roomID)).
			Str("encryption_event_json", string(encryptionEventJSON)).
			Msg("Failed to unmarshal encryption event")
		return nil, err
	}
	return &encryptionEvent, nil
}
