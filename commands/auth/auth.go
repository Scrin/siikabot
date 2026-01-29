package auth

import (
	"context"
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
		matrix.SendMessage(roomID, "Usage: !auth <challenge> or !auth logout")
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
		matrix.SendMessage(roomID, "Failed to log out from web interface")
		return
	}

	log.Info().Ctx(ctx).Str("sender", sender).Msg("User logged out from web")
	matrix.SendMessage(roomID, "Logged out from web interface")
}

// handleAuthenticate completes a web authentication challenge
func handleAuthenticate(ctx context.Context, roomID, sender, challenge string) {
	log.Debug().Ctx(ctx).Str("room_id", roomID).Str("sender", sender).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Processing auth challenge")

	// Generate a new session token
	token, err := auth.GenerateSessionToken()
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("sender", sender).Msg("Failed to generate session token")
		matrix.SendMessage(roomID, "Failed to authenticate: internal error")
		return
	}

	// Store the token in the database (this invalidates any previous session)
	err = db.SetWebSessionToken(ctx, sender, token)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("sender", sender).Msg("Failed to store session token")
		matrix.SendMessage(roomID, "Failed to authenticate: internal error")
		return
	}

	// Complete the challenge so the web UI can retrieve the token
	err = auth.CompleteChallenge(ctx, challenge, sender, token)
	if err != nil {
		// The token is already stored in the DB, but we couldn't complete the challenge
		// This might happen if the challenge expired or was invalid
		// Clear the token since the auth flow failed
		_ = db.ClearWebSessionToken(ctx, sender)
		log.Warn().Ctx(ctx).Err(err).Str("sender", sender).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Failed to complete auth challenge")
		matrix.SendMessage(roomID, "Authentication failed: invalid or expired challenge")
		return
	}

	log.Info().Ctx(ctx).Str("sender", sender).Msg("User authenticated for web")
	matrix.SendMessage(roomID, "Authenticated for web access")
}
