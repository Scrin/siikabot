package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

// RoomConfig represents a room configuration
type RoomConfig struct {
	RoomID            string `db:"room_id"`
	ChatLLMModelText  string `db:"chat_llm_model_text"`
	ChatLLMModelImage string `db:"chat_llm_model_image"`
}

// GetRoomChatLLMModelText retrieves the chat LLM model for text messages in a room
// Returns empty string if not set
func GetRoomChatLLMModelText(ctx context.Context, roomID string) (string, error) {
	var model string
	err := pool.QueryRow(ctx,
		"SELECT chat_llm_model_text FROM room_config WHERE room_id = $1",
		roomID).Scan(&model)
	if err != nil {
		// If no rows found, return empty string (no custom model set)
		if err.Error() == "no rows in result set" {
			return "", nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat LLM model for text")
		return "", err
	}
	return model, nil
}

// GetRoomChatLLMModelImage retrieves the chat LLM model for image messages in a room
// Returns empty string if not set
func GetRoomChatLLMModelImage(ctx context.Context, roomID string) (string, error) {
	var model string
	err := pool.QueryRow(ctx,
		"SELECT chat_llm_model_image FROM room_config WHERE room_id = $1",
		roomID).Scan(&model)
	if err != nil {
		// If no rows found, return empty string (no custom model set)
		if err.Error() == "no rows in result set" {
			return "", nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat LLM model for image")
		return "", err
	}
	return model, nil
}

// SetRoomChatLLMModelText sets the chat LLM model for text messages in a room
func SetRoomChatLLMModelText(ctx context.Context, roomID, model string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_llm_model_text) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_llm_model_text = $2",
		roomID, model)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Failed to set room chat LLM model for text")
		return err
	}
	return nil
}

// SetRoomChatLLMModelImage sets the chat LLM model for image messages in a room
func SetRoomChatLLMModelImage(ctx context.Context, roomID, model string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_llm_model_image) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_llm_model_image = $2",
		roomID, model)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("model", model).
			Msg("Failed to set room chat LLM model for image")
		return err
	}
	return nil
}
