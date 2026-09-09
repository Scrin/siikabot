package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/rs/zerolog/log"
)

// RoomConfig represents a room configuration
type RoomConfig struct {
	RoomID                string  `db:"room_id"`
	ChatLLMModelText      *string `db:"chat_llm_model_text"`
	ChatLLMModelImage     *string `db:"chat_llm_model_image"`
	ChatContextHighTokens *int    `db:"chat_context_high_tokens"`
	ChatContextLowTokens  *int    `db:"chat_context_low_tokens"`
	ChatContextAnchorID   *int64  `db:"chat_context_anchor_id"`
	ChatMaxToolIterations *int    `db:"chat_max_tool_iterations"`
	ChatMaxTokens         *int    `db:"chat_max_tokens"`
	ChatImageDetail       *string `db:"chat_image_detail"`
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

// GetRoomChatContextTokens retrieves the context window token budget for a room
// Returns nil values if not set (which means use the defaults)
func GetRoomChatContextTokens(ctx context.Context, roomID string) (high, low *int, err error) {
	err = pool.QueryRow(ctx,
		"SELECT chat_context_high_tokens, chat_context_low_tokens FROM room_config WHERE room_id = $1",
		roomID).Scan(&high, &low)
	if err != nil {
		// If no rows found, no custom values are set
		if err.Error() == "no rows in result set" {
			return nil, nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat context token budget")
		return nil, nil, err
	}
	return high, low, nil
}

// GetRoomChatContextAnchor retrieves the id of the chat history row the context window starts at
// Returns nil if no anchor is set, meaning the window starts at the oldest available history
func GetRoomChatContextAnchor(ctx context.Context, roomID string) (*int64, error) {
	var anchorID *int64
	err := pool.QueryRow(ctx,
		"SELECT chat_context_anchor_id FROM room_config WHERE room_id = $1",
		roomID).Scan(&anchorID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat context anchor")
		return nil, err
	}
	return anchorID, nil
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

// GetRoomChatMaxTokens retrieves the maximum response length for a room
// Returns nil if not set (which means use the default)
func GetRoomChatMaxTokens(ctx context.Context, roomID string) (*int, error) {
	var maxTokens *int
	err := pool.QueryRow(ctx,
		"SELECT chat_max_tokens FROM room_config WHERE room_id = $1",
		roomID).Scan(&maxTokens)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat max tokens")
		return nil, err
	}
	return maxTokens, nil
}

// SetRoomChatMaxTokens sets the maximum response length for a room
func SetRoomChatMaxTokens(ctx context.Context, roomID string, maxTokens int) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_max_tokens) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_max_tokens = $2",
		roomID, maxTokens)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_tokens", maxTokens).
			Msg("Failed to set room chat max tokens")
		return err
	}
	return nil
}

// GetRoomChatImageDetail retrieves the image detail level for a room
// Returns nil if not set (which means use the default)
func GetRoomChatImageDetail(ctx context.Context, roomID string) (*string, error) {
	var detail *string
	err := pool.QueryRow(ctx,
		"SELECT chat_image_detail FROM room_config WHERE room_id = $1",
		roomID).Scan(&detail)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room chat image detail")
		return nil, err
	}
	return detail, nil
}

// SetRoomChatImageDetail sets the image detail level for a room
func SetRoomChatImageDetail(ctx context.Context, roomID, detail string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_image_detail) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_image_detail = $2",
		roomID, detail)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("image_detail", detail).
			Msg("Failed to set room chat image detail")
		return err
	}
	return nil
}

// SetRoomChatContextTokens sets the context window token budget for a room
func SetRoomChatContextTokens(ctx context.Context, roomID string, high, low int) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_context_high_tokens, chat_context_low_tokens) VALUES ($1, $2, $3) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_context_high_tokens = $2, chat_context_low_tokens = $3",
		roomID, high, low)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("high_tokens", high).
			Int("low_tokens", low).
			Msg("Failed to set room chat context token budget")
		return err
	}
	return nil
}

// SetRoomChatContextAnchor sets the id of the chat history row the context window starts at
func SetRoomChatContextAnchor(ctx context.Context, roomID string, anchorID int64) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO room_config (room_id, chat_context_anchor_id) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET chat_context_anchor_id = $2",
		roomID, anchorID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int64("anchor_id", anchorID).
			Msg("Failed to set room chat context anchor")
		return err
	}
	return nil
}

// ClearRoomChatContextAnchor removes the context window anchor for a room, so the window starts
// again at the oldest available history
func ClearRoomChatContextAnchor(ctx context.Context, roomID string) error {
	_, err := pool.Exec(ctx,
		"UPDATE room_config SET chat_context_anchor_id = NULL WHERE room_id = $1",
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to clear room chat context anchor")
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

// GetRoomChatMaxWebContentSize returns the maximum web content size in bytes for a room
func GetRoomChatMaxWebContentSize(ctx context.Context, roomID string) (*int, error) {
	var maxSize *int
	err := pool.QueryRow(ctx, `
		SELECT chat_max_web_content_size
		FROM room_config
		WHERE room_id = $1
	`, roomID).Scan(&maxSize)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil && err != sql.ErrNoRows {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get room chat max web content size")
		return nil, fmt.Errorf("failed to get room chat max web content size: %w", err)
	}
	return maxSize, nil
}

// SetRoomChatMaxWebContentSize sets the maximum web content size in bytes for a room
func SetRoomChatMaxWebContentSize(ctx context.Context, roomID string, maxSize int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO room_config (room_id, chat_max_web_content_size)
		VALUES ($1, $2)
		ON CONFLICT (room_id)
		DO UPDATE SET chat_max_web_content_size = $2
	`, roomID, maxSize)

	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_size", maxSize).
			Msg("Failed to set room chat max web content size")
		return fmt.Errorf("failed to set room chat max web content size: %w", err)
	}
	return nil
}
