package db

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

var ErrInvalidToken = errors.New("invalid or expired token")

// SetWebSessionToken sets the web session token for a user.
// This will create the user_authorizations row if it doesn't exist,
// or update the existing token (invalidating any previous session).
func SetWebSessionToken(ctx context.Context, userID, token string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO user_authorizations (user_id, web_session_token)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET web_session_token = $2
	`, userID, token)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to set web session token")
		return err
	}
	return nil
}

// ClearWebSessionToken clears the web session token for a user (logout).
func ClearWebSessionToken(ctx context.Context, userID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE user_authorizations SET web_session_token = NULL WHERE user_id = $1
	`, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to clear web session token")
		return err
	}
	return nil
}

// GetUserByWebSessionToken validates a token and returns the associated user with authorizations.
// Uses constant-time comparison to prevent timing attacks.
func GetUserByWebSessionToken(ctx context.Context, token string) (User, error) {
	rows, err := pool.Query(ctx, `
		SELECT user_id, web_session_token, grafana FROM user_authorizations WHERE web_session_token IS NOT NULL
	`)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query web session tokens")
		return User{}, err
	}
	defer rows.Close()

	// Iterate through all tokens using constant-time comparison
	for rows.Next() {
		var userID, storedToken string
		var grafana bool
		if err := rows.Scan(&userID, &storedToken, &grafana); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to scan web session token row")
			continue
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(token), []byte(storedToken)) == 1 {
			return User{
				UserID: userID,
				Authorizations: UserAuthorizations{
					Grafana: grafana,
				},
			}, nil
		}
	}

	if err := rows.Err(); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Error iterating web session token rows")
		return User{}, err
	}

	return User{}, ErrInvalidToken
}

// GetWebSessionToken retrieves the current web session token for a user.
// Returns empty string if no token is set.
func GetWebSessionToken(ctx context.Context, userID string) (string, error) {
	var token *string
	err := pool.QueryRow(ctx, `
		SELECT web_session_token FROM user_authorizations WHERE user_id = $1
	`, userID).Scan(&token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to get web session token")
		return "", err
	}
	if token == nil {
		return "", nil
	}
	return *token, nil
}

// UserAuthorizations represents the authorization flags for a user
type UserAuthorizations struct {
	Grafana bool `db:"grafana"`
}

// User represents a user with their authorizations
type User struct {
	UserID         string
	Authorizations UserAuthorizations
}

// GetUserAuthorizations retrieves the authorization flags for a user.
// Returns default (all false) if the user doesn't exist in the table.
func GetUserAuthorizations(ctx context.Context, userID string) (UserAuthorizations, error) {
	var auth UserAuthorizations
	err := pool.QueryRow(ctx, `
		SELECT grafana FROM user_authorizations WHERE user_id = $1
	`, userID).Scan(&auth.Grafana)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// User doesn't exist in table yet, return defaults
			return UserAuthorizations{Grafana: false}, nil
		}
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to get user authorizations")
		return UserAuthorizations{}, err
	}
	return auth, nil
}
