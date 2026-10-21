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
//
// Each column is a chat setting, and its name is the key the setting goes by everywhere else: in
// !chat config and !chat set, and in the logs.
type ChatConfig struct {
	ID                int64         `db:"id"`
	CreatedAt         time.Time     `db:"created_at"`
	Model             string        `db:"model"`
	ContextHighTokens int           `db:"context_high_tokens"`
	ContextLowTokens  int           `db:"context_low_tokens"`
	MaxToolIterations int           `db:"max_tool_iterations"`
	MaxResponseTokens int           `db:"max_response_tokens"`
	ImageDetail       string        `db:"image_detail"`
	WebPartBytes      int           `db:"web_part_bytes"`
	TurnTimeout       time.Duration `db:"turn_timeout"`
	ModelTimeout      time.Duration `db:"model_timeout"`
	ModelAttempts     int           `db:"model_attempts"`
}

// chatConfigColumns are the columns of a ChatConfig, as every query of the table reads them
const chatConfigColumns = `id, created_at, model, context_high_tokens, context_low_tokens,
	max_tool_iterations, max_response_tokens, image_detail, web_part_bytes,
	turn_timeout, model_timeout, model_attempts`

// GetChatConfig returns the chat configuration currently in effect
func GetChatConfig(ctx context.Context) (ChatConfig, error) {
	cfg, err := getLatestChatConfig(ctx, pool.Query)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get chat config")
		return ChatConfig{}, err
	}
	return cfg, nil
}

// GetChatConfigHistory returns every chat configuration on record, newest first: the one in effect,
// then the ones it replaced. A row is added only when the admin changes a setting, so there are few.
// There is always at least one, like for GetChatConfig.
func GetChatConfigHistory(ctx context.Context) ([]ChatConfig, error) {
	rows, err := pool.Query(ctx, `SELECT `+chatConfigColumns+` FROM chat_config ORDER BY id DESC`)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query chat config history")
		return nil, err
	}
	history, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatConfig])
	if err == nil && len(history) == 0 {
		// The migration that creates the table seeds its first row and nothing deletes rows, so
		// reaching this takes a manual edit
		err = errors.New("chat_config has no rows")
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to collect chat config history")
		return nil, err
	}
	return history, nil
}

// GetChatConfigByID returns the chat configuration with the id, and whether there is one
func GetChatConfigByID(ctx context.Context, id int64) (ChatConfig, bool, error) {
	rows, err := pool.Query(ctx, `SELECT `+chatConfigColumns+` FROM chat_config WHERE id = $1`, id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("config_id", id).Msg("Failed to query chat config")
		return ChatConfig{}, false, err
	}
	cfg, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[ChatConfig])
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatConfig{}, false, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("config_id", id).Msg("Failed to collect chat config")
		return ChatConfig{}, false, err
	}
	return cfg, true, nil
}

// UpdateChatConfig applies a change to the chat configuration and returns the result. The table is
// append-only, so the change is applied to a copy of the latest row and inserted as a new row,
// leaving every earlier configuration on record.
//
// The change can refuse to apply, by returning an error. Nothing is written then, and the error is
// returned as it is, for the caller to tell apart from a failure of the database.
//
// The copy and the insert happen under a table lock. Each command runs in its own goroutine, and two
// changes made at the same moment would otherwise both copy the same row, the second silently
// undoing the first. Checks that the change makes run under the lock as well, on the row as it then
// is. The lock only holds off other writers: plain reads are not blocked by it.
func UpdateChatConfig(ctx context.Context, change func(*ChatConfig) error) (ChatConfig, error) {
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

	if err := change(&cfg); err != nil {
		return ChatConfig{}, err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO chat_config
			(model, context_high_tokens, context_low_tokens,
			 max_tool_iterations, max_response_tokens, image_detail, web_part_bytes,
			 turn_timeout, model_timeout, model_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		cfg.Model, cfg.ContextHighTokens, cfg.ContextLowTokens,
		cfg.MaxToolIterations, cfg.MaxResponseTokens, cfg.ImageDetail, cfg.WebPartBytes,
		cfg.TurnTimeout, cfg.ModelTimeout, cfg.ModelAttempts,
	).Scan(&cfg.ID, &cfg.CreatedAt)
	if err != nil {
		// Includes a change the table's checks reject, which leaves the configuration as it was. The
		// caller logs the settings it tried to change.
		log.Error().Ctx(ctx).Err(err).Int64("based_on_config_id", cfg.ID).Msg("Failed to insert chat config")
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
	rows, err := query(ctx, `SELECT `+chatConfigColumns+` FROM chat_config ORDER BY id DESC LIMIT 1`)
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
