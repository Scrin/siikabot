package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/auth"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

// ChallengeResponse is the response for the challenge endpoint
type ChallengeResponse struct {
	Challenge  string `json:"challenge"`   // Public - shown to user, sent on Matrix
	PollSecret string `json:"poll_secret"` // Private - only for polling, never shown to user
	ExpiresAt  string `json:"expires_at"`
}

// PollResponse is the response for the poll endpoint
type PollResponse struct {
	Status string `json:"status"` // "pending" or "authenticated"
	Token  string `json:"token,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// MeResponse is the response for the me endpoint
type MeResponse struct {
	UserID string `json:"user_id"`
}

// ErrorResponse is a generic error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ChallengeHandler generates a new authentication challenge
// POST /api/auth/challenge
func ChallengeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	challenge, pollSecret, expiresAt, err := auth.GenerateChallenge(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate auth challenge")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Too many pending authentication requests"})
		return
	}

	response := ChallengeResponse{
		Challenge:  challenge,
		PollSecret: pollSecret,
		ExpiresAt:  expiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode challenge response")
	}
}

// PollHandler checks if a challenge has been completed
// GET /api/auth/poll?challenge=xxx&poll_secret=yyy
func PollHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	challenge := r.URL.Query().Get("challenge")
	pollSecret := r.URL.Query().Get("poll_secret")
	if challenge == "" || pollSecret == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Missing challenge or poll_secret parameter"})
		return
	}

	// Try to consume the result (this removes it from the store)
	token, userID, err := auth.ConsumeChallengeResult(ctx, challenge, pollSecret)
	if err == nil {
		response := PollResponse{
			Status: "authenticated",
			Token:  token,
			UserID: userID,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to encode poll response")
		}
		return
	}

	// Check if the error is invalid poll secret (potential attack)
	if err == auth.ErrInvalidPollSecret {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid poll secret"})
		return
	}

	// Check if the challenge is still pending (not completed yet)
	completed, _, _, pollErr := auth.PollChallenge(ctx, challenge, pollSecret)
	if pollErr == auth.ErrInvalidPollSecret {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid poll secret"})
		return
	}
	if completed {
		// Shouldn't happen since ConsumeChallengeResult would have caught it
		// but handle it gracefully
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Challenge already consumed"})
		return
	}

	// Challenge exists but not yet completed, or challenge doesn't exist
	// We don't distinguish to avoid leaking information about valid challenges
	response := PollResponse{
		Status: "pending",
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode poll response")
	}
}

// MeHandler returns the current authenticated user
// GET /api/auth/me
// Requires Authorization: Bearer <token> header
func MeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Missing or invalid Authorization header"})
		return
	}

	userID, err := db.GetUserByWebSessionToken(ctx, token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid or expired token"})
		return
	}

	response := MeResponse{
		UserID: userID,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode me response")
	}
}

// LogoutHandler clears the user's web session token
// POST /api/auth/logout
// Requires Authorization: Bearer <token> header
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Missing or invalid Authorization header"})
		return
	}

	// Get user ID from token
	userID, err := db.GetUserByWebSessionToken(ctx, token)
	if err != nil {
		// Token already invalid, consider logout successful
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	// Clear the token from DB
	if err := db.ClearWebSessionToken(ctx, userID); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to clear web session token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to logout"})
		return
	}

	log.Info().Ctx(ctx).Str("user_id", userID).Msg("User logged out from web via API")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// extractBearerToken extracts the token from the Authorization header
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}

// AuthMiddleware creates middleware that validates the auth token and adds the user ID to the context
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		token := extractBearerToken(r)
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Missing or invalid Authorization header"})
			return
		}

		userID, err := db.GetUserByWebSessionToken(ctx, token)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid or expired token"})
			return
		}

		// Add user ID to context
		ctx = context.WithValue(ctx, userIDContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// contextKey is a type for context keys to avoid collisions
type contextKey string

const userIDContextKey contextKey = "user_id"

// GetUserIDFromContext retrieves the user ID from the request context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}
