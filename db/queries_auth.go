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

// GetUserByWebSessionToken validates a token and returns the associated user ID.
// Uses constant-time comparison to prevent timing attacks.
func GetUserByWebSessionToken(ctx context.Context, token string) (string, error) {
	rows, err := pool.Query(ctx, `
		SELECT user_id, web_session_token FROM user_authorizations WHERE web_session_token IS NOT NULL
	`)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query web session tokens")
		return "", err
	}
	defer rows.Close()

	// Iterate through all tokens using constant-time comparison
	for rows.Next() {
		var userID, storedToken string
		if err := rows.Scan(&userID, &storedToken); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to scan web session token row")
			continue
		}

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(token), []byte(storedToken)) == 1 {
			return userID, nil
		}
	}

	if err := rows.Err(); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Error iterating web session token rows")
		return "", err
	}

	return "", ErrInvalidToken
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
