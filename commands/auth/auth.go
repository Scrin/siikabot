package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/Scrin/siikabot/auth"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// Handle handles the !auth command
// Usage:
//   - !auth <challenge> - Complete web authentication
//   - !auth logout - Log out from web interface
func Handle(ctx context.Context, roomID, sender, msg string) {
	split := strings.Fields(msg)
	if len(split) < 2 {
		matrix.SendMessage(ctx, roomID, "Usage: !auth <challenge> or !auth logout")
		return
	}

	arg := split[1]

	if strings.ToLower(arg) == "logout" {
		handleLogout(ctx, roomID, sender)
		return
	}

	handleAuthenticate(ctx, roomID, sender, arg)
}

// handleLogout clears the user's web session token
func handleLogout(ctx context.Context, roomID, sender string) {
	log.Debug().Ctx(ctx).Str("room_id", roomID).Str("sender", sender).Msg("Processing auth logout")

	err := db.ClearWebSessionToken(ctx, sender)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("sender", sender).Msg("Failed to clear web session token")
		matrix.SendMessage(ctx, roomID, "Failed to log out from web interface")
		return
	}

	log.Info().Ctx(ctx).Str("sender", sender).Msg("User logged out from web")
	matrix.SendMessage(ctx, roomID, "Logged out from web interface")
}

// handleAuthenticate completes a web authentication challenge
func handleAuthenticate(ctx context.Context, roomID, sender, challenge string) {
	log.Debug().Ctx(ctx).Str("room_id", roomID).Str("sender", sender).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Processing auth challenge")

	// The session token is made only for a challenge that checks out, so a mistyped or expired one leaves
	// the session the sender already has alone. Storing the new token ends any previous session.
	err := auth.CompleteChallenge(ctx, challenge, sender, func() (string, error) {
		token, err := auth.GenerateSessionToken()
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("sender", sender).Msg("Failed to generate session token")
			return "", err
		}
		if err := db.SetWebSessionToken(ctx, sender, token); err != nil {
			// Already logged
			return "", err
		}
		return token, nil
	})
	switch {
	case err == nil:
		log.Info().Ctx(ctx).Str("sender", sender).Msg("User authenticated for web")
		matrix.SendMessage(ctx, roomID, "Authenticated for web access")
	case errors.Is(err, auth.ErrChallengeNotFound):
		log.Info().Ctx(ctx).Str("sender", sender).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Auth challenge not found or expired")
		matrix.SendMessage(ctx, roomID, "Authentication failed: invalid or expired challenge")
	case errors.Is(err, auth.ErrChallengeCompleted):
		matrix.SendMessage(ctx, roomID, "Authentication failed: this challenge has already been used. Start the login again in your browser.")
	default:
		log.Error().Ctx(ctx).Err(err).Str("sender", sender).Msg("Failed to complete auth challenge")
		matrix.SendMessage(ctx, roomID, "Failed to authenticate: internal error")
	}
}
