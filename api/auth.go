package api

import (
	"net/http"
	"strings"

	"github.com/Scrin/siikabot/auth"
	"github.com/Scrin/siikabot/db"
	"github.com/gin-gonic/gin"
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
	UserID         string         `json:"user_id"`
	Authorizations Authorizations `json:"authorizations"`
}

// Authorizations represents user permission flags
type Authorizations struct {
	Grafana bool `json:"grafana"`
}

// ErrorResponse is a generic error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ChallengeHandler generates a new authentication challenge
// POST /api/auth/challenge
func ChallengeHandler(c *gin.Context) {
	ctx := c.Request.Context()

	challenge, pollSecret, expiresAt, err := auth.GenerateChallenge(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate auth challenge")
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "Too many pending authentication requests"})
		return
	}

	c.JSON(http.StatusOK, ChallengeResponse{
		Challenge:  challenge,
		PollSecret: pollSecret,
		ExpiresAt:  expiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// PollHandler checks if a challenge has been completed
// GET /api/auth/poll?challenge=xxx&poll_secret=yyy
func PollHandler(c *gin.Context) {
	ctx := c.Request.Context()

	challenge := c.Query("challenge")
	pollSecret := c.Query("poll_secret")
	if challenge == "" || pollSecret == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing challenge or poll_secret parameter"})
		return
	}

	// Try to consume the result (this removes it from the store)
	token, userID, err := auth.ConsumeChallengeResult(ctx, challenge, pollSecret)
	if err == nil {
		c.JSON(http.StatusOK, PollResponse{
			Status: "authenticated",
			Token:  token,
			UserID: userID,
		})
		return
	}

	// Check if the error is invalid poll secret (potential attack)
	if err == auth.ErrInvalidPollSecret {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Invalid poll secret"})
		return
	}

	// Check if the challenge is still pending (not completed yet)
	completed, _, _, pollErr := auth.PollChallenge(ctx, challenge, pollSecret)
	if pollErr == auth.ErrInvalidPollSecret {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Invalid poll secret"})
		return
	}
	if completed {
		// Shouldn't happen since ConsumeChallengeResult would have caught it
		// but handle it gracefully
		c.JSON(http.StatusGone, ErrorResponse{Error: "Challenge already consumed"})
		return
	}

	// Challenge exists but not yet completed, or challenge doesn't exist
	// We don't distinguish to avoid leaking information about valid challenges
	c.JSON(http.StatusOK, PollResponse{Status: "pending"})
}

// MeHandler returns the current authenticated user
// GET /api/auth/me
// Requires Authorization: Bearer <token> header
func MeHandler(c *gin.Context) {
	ctx := c.Request.Context()

	token := extractBearerToken(c.Request)
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Missing or invalid Authorization header"})
		return
	}

	user, err := db.GetUserByWebSessionToken(ctx, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid or expired token"})
		return
	}

	c.JSON(http.StatusOK, MeResponse{
		UserID: user.UserID,
		Authorizations: Authorizations{
			Grafana: user.Authorizations.Grafana,
		},
	})
}

// LogoutHandler clears the user's web session token
// POST /api/auth/logout
// Requires Authorization: Bearer <token> header
func LogoutHandler(c *gin.Context) {
	ctx := c.Request.Context()

	token := extractBearerToken(c.Request)
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Missing or invalid Authorization header"})
		return
	}

	// Get user ID from token
	user, err := db.GetUserByWebSessionToken(ctx, token)
	if err != nil {
		// Token already invalid, consider logout successful
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}

	// Clear the token from DB
	if err := db.ClearWebSessionToken(ctx, user.UserID); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", user.UserID).Msg("Failed to clear web session token")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to logout"})
		return
	}

	log.Info().Ctx(ctx).Str("user_id", user.UserID).Msg("User logged out from web via API")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

// AuthMiddleware creates Gin middleware that validates the auth token and adds the user ID to the context
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		token := extractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "Missing or invalid Authorization header"})
			return
		}

		user, err := db.GetUserByWebSessionToken(ctx, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid or expired token"})
			return
		}

		// Add user ID to Gin context
		c.Set("user_id", user.UserID)
		c.Next()
	}
}

// GetUserIDFromContext retrieves the user ID from the Gin context
func GetUserIDFromContext(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return "", false
	}
	return userID.(string), true
}
