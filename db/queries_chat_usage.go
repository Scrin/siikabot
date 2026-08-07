package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ChatUsage is the record of a single chat turn's cost
type ChatUsage struct {
	ID                 int64     `db:"id"`
	Timestamp          time.Time `db:"timestamp"`
	RoomID             string    `db:"room_id"`
	UserID             string    `db:"user_id"`
	Model              string    `db:"model"`
	PromptTokens       int       `db:"prompt_tokens"`
	CompletionTokens   int       `db:"completion_tokens"`
	CachedPromptTokens int       `db:"cached_prompt_tokens"`
	ToolIterations     int       `db:"tool_iterations"`
	HasImage           bool      `db:"has_image"`
	DurationMS         int       `db:"duration_ms"`
	Outcome            string    `db:"outcome"`
}

// ChatUsageSummary aggregates chat usage over a period, grouped by room and model
type ChatUsageSummary struct {
	RoomID             string `db:"room_id"`
	Model              string `db:"model"`
	Turns              int    `db:"turns"`
	PromptTokens       int    `db:"prompt_tokens"`
	CompletionTokens   int    `db:"completion_tokens"`
	CachedPromptTokens int    `db:"cached_prompt_tokens"`
	ToolIterations     int    `db:"tool_iterations"`
	Failures           int    `db:"failures"`
}

// SaveChatUsage records the cost of a completed chat turn
func SaveChatUsage(ctx context.Context, usage ChatUsage) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO chat_usage
			(room_id, user_id, model, prompt_tokens, completion_tokens, cached_prompt_tokens,
			 tool_iterations, has_image, duration_ms, outcome)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		usage.RoomID, usage.UserID, usage.Model, usage.PromptTokens, usage.CompletionTokens,
		usage.CachedPromptTokens, usage.ToolIterations, usage.HasImage, usage.DurationMS, usage.Outcome)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", usage.RoomID).
			Str("model", usage.Model).
			Msg("Failed to save chat usage")
		return err
	}
	return nil
}

// GetChatUsageSummary aggregates chat usage since the given time, grouped by room and model,
// most expensive first
func GetChatUsageSummary(ctx context.Context, since time.Time) ([]ChatUsageSummary, error) {
	rows, err := pool.Query(ctx,
		`SELECT room_id, model,
			COUNT(*)::int AS turns,
			COALESCE(SUM(prompt_tokens), 0)::int AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0)::int AS completion_tokens,
			COALESCE(SUM(cached_prompt_tokens), 0)::int AS cached_prompt_tokens,
			COALESCE(SUM(tool_iterations), 0)::int AS tool_iterations,
			COUNT(*) FILTER (WHERE outcome <> 'ok')::int AS failures
		FROM chat_usage
		WHERE timestamp >= $1
		GROUP BY room_id, model
		ORDER BY prompt_tokens + completion_tokens DESC`,
		since)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Time("since", since).
			Msg("Failed to query chat usage summary")
		return nil, err
	}

	summaries, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatUsageSummary])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to collect chat usage summary rows")
		return nil, err
	}
	return summaries, nil
}

// CleanupOldChatUsage removes usage records older than the specified duration
func CleanupOldChatUsage(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)

	tag, err := pool.Exec(ctx,
		"DELETE FROM chat_usage WHERE timestamp < $1",
		cutoffTime)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Time("cutoff_time", cutoffTime).
			Msg("Failed to cleanup old chat usage")
		return 0, err
	}

	return tag.RowsAffected(), nil
}
