package db

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// ErrWebhookNameTaken is returned when another webhook already has the name, ignoring case
var ErrWebhookNameTaken = errors.New("webhook name is already in use")

// ErrWebhookNotFound is returned for a webhook that doesn't exist
var ErrWebhookNotFound = errors.New("webhook not found")

// Webhook posts messages into a room the bot is in. Everything but the ID can be changed.
type Webhook struct {
	ID     string `db:"id"`
	Name   string `db:"name"`
	RoomID string `db:"room_id"`
	// TokenHash is the SHA-256 of the bearer token, which isn't stored
	TokenHash []byte    `db:"token_hash"`
	CreatedAt time.Time `db:"created_at"`
	// LastUsedAt is when a message was last delivered, nil if none has been
	LastUsedAt *time.Time `db:"last_used_at"`
}

// WebhookChanges are changes to a webhook, nil for what stays as it is
type WebhookChanges struct {
	Name      *string
	RoomID    *string
	TokenHash []byte
}

const webhookColumns = "id, name, room_id, token_hash, created_at, last_used_at"

// isWebhookNameTaken reports whether an error is a webhook getting a name another one has
func isWebhookNameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "webhooks_name_idx"
}

// ListWebhooks returns every webhook, ordered by name
func ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := pool.Query(ctx, "SELECT "+webhookColumns+" FROM webhooks ORDER BY lower(name)")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query webhooks")
		return nil, err
	}
	webhooks, err := pgx.CollectRows(rows, pgx.RowToStructByName[Webhook])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to read webhooks")
		return nil, err
	}
	return webhooks, nil
}

// GetWebhook returns a webhook, and whether it exists
func GetWebhook(ctx context.Context, id string) (Webhook, bool, error) {
	rows, err := pool.Query(ctx, "SELECT "+webhookColumns+" FROM webhooks WHERE id = $1", id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("webhook_id", id).Msg("Failed to query a webhook")
		return Webhook{}, false, err
	}
	webhook, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Webhook])
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, false, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("webhook_id", id).Msg("Failed to read a webhook")
		return Webhook{}, false, err
	}
	return webhook, true, nil
}

// CreateWebhook stores a new webhook, and returns it as stored. Returns ErrWebhookNameTaken if
// another webhook has the name.
func CreateWebhook(ctx context.Context, webhook Webhook) (Webhook, error) {
	rows, err := pool.Query(ctx,
		"INSERT INTO webhooks (id, name, room_id, token_hash) VALUES ($1, $2, $3, $4) RETURNING "+webhookColumns,
		webhook.ID, webhook.Name, webhook.RoomID, webhook.TokenHash)
	var created Webhook
	if err == nil {
		created, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Webhook])
	}
	if isWebhookNameTaken(err) {
		return Webhook{}, ErrWebhookNameTaken
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("webhook_id", webhook.ID).
			Str("webhook_name", webhook.Name).
			Str("room_id", webhook.RoomID).
			Msg("Failed to create a webhook")
		return Webhook{}, err
	}
	return created, nil
}

// UpdateWebhook makes the given changes to a webhook, and returns it as it now is. Returns
// ErrWebhookNotFound if there is no such webhook, and ErrWebhookNameTaken if another webhook has the
// new name.
func UpdateWebhook(ctx context.Context, id string, changes WebhookChanges) (Webhook, error) {
	rows, err := pool.Query(ctx,
		`UPDATE webhooks SET
			name = COALESCE($2, name),
			room_id = COALESCE($3, room_id),
			token_hash = COALESCE($4, token_hash)
		WHERE id = $1 RETURNING `+webhookColumns,
		id, changes.Name, changes.RoomID, changes.TokenHash)
	var webhook Webhook
	if err == nil {
		webhook, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Webhook])
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrWebhookNotFound
	}
	if isWebhookNameTaken(err) {
		return Webhook{}, ErrWebhookNameTaken
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("webhook_id", id).
			Bool("name_changed", changes.Name != nil).
			Bool("room_changed", changes.RoomID != nil).
			Bool("token_changed", changes.TokenHash != nil).
			Msg("Failed to update a webhook")
		return Webhook{}, err
	}
	return webhook, nil
}

// DeleteWebhook removes a webhook, and reports whether there was one to remove
func DeleteWebhook(ctx context.Context, id string) (bool, error) {
	tag, err := pool.Exec(ctx, "DELETE FROM webhooks WHERE id = $1", id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("webhook_id", id).Msg("Failed to delete a webhook")
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// TouchWebhook records that a webhook has just delivered a message
func TouchWebhook(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, "UPDATE webhooks SET last_used_at = now() WHERE id = $1", id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("webhook_id", id).Msg("Failed to record a webhook's last use")
	}
	return err
}
