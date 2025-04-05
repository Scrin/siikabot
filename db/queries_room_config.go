package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// RoomConfig represents a room configuration
type RoomConfig struct {
	RoomID                 string  `db:"room_id"`
	ChatLLMModelText       *string `db:"chat_llm_model_text"`
	ChatLLMModelImage      *string `db:"chat_llm_model_image"`
	ChatMaxHistoryMessages *int    `db:"chat_max_history_messages"`
	ChatMaxToolIterations  *int    `db:"chat_max_tool_iterations"`
}

// GetRoomChatLLMModelText retrieves the chat LLM model for text messages in a room
// Returns empty string if not set
func GetRoomChatLLMModelText(ctx context.Context, roomID string) (*string, error) {
	var model *string
	err := pool.QueryRow(ctx,
		"SELECT chat_llm_model_text FROM room_config WHERE room_id = $1",
		roomID).Scan(&model)
	if err != nil {
		// If no rows found, return empty string (no custom model set)
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat LLM model for text")
		return nil, err
	}
	return model, nil
}

// GetRoomChatLLMModelImage retrieves the chat LLM model for image messages in a room
// Returns empty string if not set
func GetRoomChatLLMModelImage(ctx context.Context, roomID string) (*string, error) {
	var model *string
	err := pool.QueryRow(ctx,
		"SELECT chat_llm_model_image FROM room_config WHERE room_id = $1",
		roomID).Scan(&model)
	if err != nil {
		// If no rows found, return empty string (no custom model set)
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat LLM model for image")
		return nil, err
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

// GetRoomChatMaxHistoryMessages retrieves the max history messages for a room
// Returns 0 if not set (which means use the default)
func GetRoomChatMaxHistoryMessages(ctx context.Context, roomID string) (*int, error) {
	var maxMessages *int
	err := pool.QueryRow(ctx,
		"SELECT chat_max_history_messages FROM room_config WHERE room_id = $1",
		roomID).Scan(&maxMessages)
	if err != nil {
		// If no rows found, return 0 (no custom value set)
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat max history messages")
		return nil, err
	}
	return maxMessages, nil
}

// GetRoomChatMaxToolIterations retrieves the max tool iterations for a room
// Returns 0 if not set (which means use the default)
func GetRoomChatMaxToolIterations(ctx context.Context, roomID string) (*int, error) {
	var maxIterations *int
	err := pool.QueryRow(ctx,
		"SELECT chat_max_tool_iterations FROM room_config WHERE room_id = $1",
		roomID).Scan(&maxIterations)
	if err != nil {
		// If no rows found, return 0 (no custom value set)
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat max tool iterations")
		return nil, err
	}
	return maxIterations, nil
}

// SetRoomChatMaxHistoryMessages sets the max history messages for a room
func SetRoomChatMaxHistoryMessages(ctx context.Context, roomID string, maxMessages int) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_max_history_messages) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_max_history_messages = $2",
		roomID, maxMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_messages", maxMessages).
			Msg("Failed to set room chat max history messages")
		return err
	}
	return nil
}

// SetRoomChatMaxToolIterations sets the max tool iterations for a room
func SetRoomChatMaxToolIterations(ctx context.Context, roomID string, maxIterations int) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_max_tool_iterations) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_max_tool_iterations = $2",
		roomID, maxIterations)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_iterations", maxIterations).
			Msg("Failed to set room chat max tool iterations")
		return err
	}
	return nil
}

// IsCommandEnabled checks if a given command is explicitly enabled in the enabled_commands array for a room
func IsCommandEnabled(ctx context.Context, roomID string, command string) (bool, error) {
	var enabledCommands []string
	err := pool.QueryRow(ctx, "SELECT enabled_commands FROM room_config WHERE room_id = $1", roomID).Scan(&enabledCommands)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	for _, enabled := range enabledCommands {
		if enabled == command {
			return true, nil
		}
	}
	return false, nil
}

// SetCommandEnabled enables or disables a command for a room by updating the enabled_commands array
func SetCommandEnabled(ctx context.Context, roomID string, command string, enabled bool) error {
	if enabled {
		// Add command to enabled_commands array if not already present
		_, err := pool.Exec(ctx, `
			INSERT INTO room_config (room_id, enabled_commands)
			VALUES ($1, ARRAY[$2])
			ON CONFLICT (room_id) DO UPDATE
			SET enabled_commands = 
				CASE 
					WHEN $2 = ANY(room_config.enabled_commands) THEN room_config.enabled_commands
					ELSE array_append(room_config.enabled_commands, $2)
				END
		`, roomID, command)
		return err
	} else {
		// Remove command from enabled_commands array
		_, err := pool.Exec(ctx, `
			UPDATE room_config
			SET enabled_commands = array_remove(enabled_commands, $2)
			WHERE room_id = $1
		`, roomID, command)
		return err
	}
}
