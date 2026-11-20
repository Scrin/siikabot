package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Scrin/siikabot/auth"
	"github.com/Scrin/siikabot/config"
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
	Admin bool `json:"admin"`
}

// ErrorResponse is a generic error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ErrorFrom is an error response that says what went wrong, written as a sentence
func ErrorFrom(err error) ErrorResponse {
	text := err.Error()
	first, size := utf8.DecodeRuneInString(text)
	return ErrorResponse{Error: string(unicode.ToUpper(first)) + text[size:]}
}

// ChallengeHandler generates a new authentication challenge
// POST /api/auth/challenge
func ChallengeHandler(c *gin.Context) {
	ctx := c.Request.Context()

	challenge, pollSecret, expiresAt, err := auth.GenerateChallenge(ctx)
	if err != nil {
		// Already logged
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create an authentication challenge"})
		return
	}

	c.JSON(http.StatusOK, ChallengeResponse{
		Challenge:  challenge,
		PollSecret: pollSecret,
		ExpiresAt:  expiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

// PollRequest is the body of a poll. The poll secret travels in the body rather than in the URL, which access
// logs keep.
type PollRequest struct {
	Challenge  string `json:"challenge"`
	PollSecret string `json:"poll_secret"`
}

// PollHandler checks if a challenge has been completed, and hands over its session token if it has
// POST /api/auth/poll with a PollRequest
func PollHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req PollRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Challenge == "" || req.PollSecret == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing challenge or poll_secret"})
		return
	}

	// Try to consume the result (this removes it from the store)
	token, userID, err := auth.ConsumeChallengeResult(ctx, req.Challenge, req.PollSecret)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, PollResponse{
			Status: "authenticated",
			Token:  token,
			UserID: userID,
		})
	case errors.Is(err, auth.ErrInvalidPollSecret):
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Invalid poll secret"})
	default:
		// Not completed yet, or expired: the browser finds out when the challenge expires
		c.JSON(http.StatusOK, PollResponse{Status: "pending"})
	}
}

// MeHandler returns the current authenticated user
// GET /api/auth/me
// Requires Authorization: Bearer <token> header
func MeHandler(c *gin.Context) {
	ctx := c.Request.Context()

	token := ExtractBearerToken(c.Request)
	if token == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Missing or invalid Authorization header"})
		return
	}

	user, err := db.GetUserByWebSessionToken(ctx, token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid or expired token"})
		return
	}

	isAdmin := user.UserID == config.Admin

	c.JSON(http.StatusOK, MeResponse{
		UserID: user.UserID,
		Authorizations: Authorizations{
			Admin: isAdmin,
		},
	})
}

// LogoutHandler clears the user's web session token
// POST /api/auth/logout
// Requires Authorization: Bearer <token> header
func LogoutHandler(c *gin.Context) {
	ctx := c.Request.Context()

	token := ExtractBearerToken(c.Request)
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

// ExtractBearerToken extracts the token from the Authorization header, or returns an empty string if
// there is none
func ExtractBearerToken(r *http.Request) string {
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

		token := ExtractBearerToken(c.Request)
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
