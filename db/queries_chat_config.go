package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ChatConfig is one row of the append-only chat configuration. The latest row is the configuration
// in effect, and the older rows are the history of what was set when.
type ChatConfig struct {
	ID                int64     `db:"id"`
	CreatedAt         time.Time `db:"created_at"`
	TextModel         string    `db:"text_model"`
	ImageModel        string    `db:"image_model"`
	ContextHighTokens int       `db:"context_high_tokens"`
	ContextLowTokens  int       `db:"context_low_tokens"`
	MaxToolIterations int       `db:"max_tool_iterations"`
	MaxTokens         int       `db:"max_tokens"`
	ImageDetail       string    `db:"image_detail"`
	MaxWebContentSize int       `db:"max_web_content_size"`
}

// GetChatConfig returns the chat configuration currently in effect
func GetChatConfig(ctx context.Context) (ChatConfig, error) {
	cfg, err := getLatestChatConfig(ctx, pool.Query)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get chat config")
		return ChatConfig{}, err
	}
	return cfg, nil
}

// UpdateChatConfig applies a change to the chat configuration and returns the result. The table is
// append-only, so the change is applied to a copy of the latest row and inserted as a new row,
// leaving every earlier configuration on record.
//
// The copy and the insert happen under a table lock. Each command runs in its own goroutine, and two
// changes made at the same moment would otherwise both copy the same row, the second silently
// undoing the first. The lock only holds off other writers: plain reads are not blocked by it.
func UpdateChatConfig(ctx context.Context, change func(*ChatConfig)) (ChatConfig, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to begin transaction for chat config update")
		return ChatConfig{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "LOCK TABLE chat_config IN EXCLUSIVE MODE"); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to lock chat config")
		return ChatConfig{}, err
	}

	cfg, err := getLatestChatConfig(ctx, tx.Query)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get chat config for update")
		return ChatConfig{}, err
	}

	change(&cfg)

	err = tx.QueryRow(ctx,
		`INSERT INTO chat_config
			(text_model, image_model, context_high_tokens, context_low_tokens,
			 max_tool_iterations, max_tokens, image_detail, max_web_content_size)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		cfg.TextModel, cfg.ImageModel, cfg.ContextHighTokens, cfg.ContextLowTokens,
		cfg.MaxToolIterations, cfg.MaxTokens, cfg.ImageDetail, cfg.MaxWebContentSize,
	).Scan(&cfg.ID, &cfg.CreatedAt)
	if err != nil {
		// Includes a change the table's checks reject, which leaves the configuration as it was
		log.Error().Ctx(ctx).Err(err).
			Str("text_model", cfg.TextModel).
			Str("image_model", cfg.ImageModel).
			Int("context_high_tokens", cfg.ContextHighTokens).
			Int("context_low_tokens", cfg.ContextLowTokens).
			Int("max_tool_iterations", cfg.MaxToolIterations).
			Int("max_tokens", cfg.MaxTokens).
			Str("image_detail", cfg.ImageDetail).
			Int("max_web_content_size", cfg.MaxWebContentSize).
			Msg("Failed to insert chat config")
		return ChatConfig{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to commit chat config update")
		return ChatConfig{}, err
	}

	return cfg, nil
}

// getLatestChatConfig reads the most recently added configuration row, with the given query
// function so it can run either on the pool or inside a transaction.
//
// Ordered by id rather than created_at: ids only ever increase, while timestamps can tie.
func getLatestChatConfig(ctx context.Context, query queryFunc) (ChatConfig, error) {
	rows, err := query(ctx,
		`SELECT id, created_at, text_model, image_model, context_high_tokens, context_low_tokens,
			max_tool_iterations, max_tokens, image_detail, max_web_content_size
		FROM chat_config
		ORDER BY id DESC
		LIMIT 1`)
	if err != nil {
		return ChatConfig{}, err
	}

	cfg, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ChatConfig])
	if errors.Is(err, pgx.ErrNoRows) {
		// There is no default to fall back to. The migration that creates the table seeds its first
		// row and nothing deletes rows, so reaching this takes a manual edit.
		return ChatConfig{}, fmt.Errorf("chat_config has no rows: %w", err)
	}
	return cfg, err
}
