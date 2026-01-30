package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// UserAuthorizationInfo represents a user with their authorization flags
type UserAuthorizationInfo struct {
	UserID  string `db:"user_id"`
	Grafana bool   `db:"grafana"`
}

// SetUserAuthorization sets or revokes a specific authorization flag for a user.
// feature: the feature name (e.g., "grafana")
// authorized: true to grant, false to revoke
func SetUserAuthorization(ctx context.Context, userID, feature string, authorized bool) error {
	var query string

	switch feature {
	case "grafana":
		if authorized {
			query = `INSERT INTO user_authorizations (user_id, grafana) VALUES ($1, true)
				ON CONFLICT (user_id) DO UPDATE SET grafana = true`
		} else {
			query = `UPDATE user_authorizations SET grafana = false WHERE user_id = $1`
		}
	default:
		return fmt.Errorf("unknown feature: %s", feature)
	}

	_, err := pool.Exec(ctx, query, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("feature", feature).
			Bool("authorized", authorized).
			Msg("Failed to set user authorization")
		return err
	}
	return nil
}

// GetAllUsersWithAuthorizations returns all users who have any authorization entry
func GetAllUsersWithAuthorizations(ctx context.Context) ([]UserAuthorizationInfo, error) {
	rows, err := pool.Query(ctx, `
		SELECT user_id, grafana FROM user_authorizations ORDER BY user_id
	`)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query user authorizations")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserAuthorizationInfo])
}

// GetSupportedFeatures returns a list of supported authorization features
func GetSupportedFeatures() []string {
	return []string{"grafana"}
}
