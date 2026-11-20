package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

var ErrInvalidToken = errors.New("invalid or expired token")

const (
	// SessionIdleTimeout is how long a dashboard session lasts unused. An open dashboard keeps using it.
	SessionIdleTimeout = 30 * 24 * time.Hour

	// sessionTouchInterval is how old a session's last use may get before a request records a new one, so
	// that not every request writes
	sessionTouchInterval = time.Hour
)

// hashSessionToken is what a session token is stored as. Only the hash is kept, so the database holds
// nothing that would log anyone in. A token is random, so a plain hash is enough.
func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SetWebSessionToken sets the web session token for a user.
// This will create the user_authorizations row if it doesn't exist,
// or update the existing token (invalidating any previous session).
func SetWebSessionToken(ctx context.Context, userID, token string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO user_authorizations (user_id, web_session_token_hash, web_session_last_used_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE
		SET web_session_token_hash = EXCLUDED.web_session_token_hash,
			web_session_last_used_at = EXCLUDED.web_session_last_used_at
	`, userID, hashSessionToken(token))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to set web session token")
		return err
	}
	return nil
}

// ClearWebSessionToken clears the web session token for a user (logout).
func ClearWebSessionToken(ctx context.Context, userID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE user_authorizations SET web_session_token_hash = NULL, web_session_last_used_at = NULL
		WHERE user_id = $1
	`, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to clear web session token")
		return err
	}
	return nil
}

// GetUserByWebSessionToken validates a token and returns the associated user. A session unused for
// SessionIdleTimeout has ended. Using one keeps it going.
func GetUserByWebSessionToken(ctx context.Context, token string) (User, error) {
	hash := hashSessionToken(token)

	var userID string
	var lastUsed, now time.Time
	err := pool.QueryRow(ctx, `
		SELECT user_id, web_session_last_used_at, now() FROM user_authorizations WHERE web_session_token_hash = $1
	`, hash).Scan(&userID, &lastUsed, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidToken
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to look up a web session token")
		return User{}, err
	}

	idle := now.Sub(lastUsed)
	if idle > SessionIdleTimeout {
		log.Info().Ctx(ctx).Str("user_id", userID).Dur("idle", idle).Msg("Web session ended after going unused")
		return User{}, ErrInvalidToken
	}
	if idle > sessionTouchInterval {
		_, err := pool.Exec(ctx, `
			UPDATE user_authorizations SET web_session_last_used_at = now()
			WHERE user_id = $1 AND web_session_token_hash = $2
		`, userID, hash)
		if err != nil {
			// The session still works; it's only recorded as used a little earlier than it was
			log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to record the use of a web session")
		}
	}

	return User{UserID: userID}, nil
}

// User represents an authenticated user
type User struct {
	UserID string
}
