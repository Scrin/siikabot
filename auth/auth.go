package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// ChallengeTTL is how long a challenge is valid
	ChallengeTTL = 1 * time.Minute

	// MaxPendingChallenges is the maximum number of pending challenges
	MaxPendingChallenges = 10000

	// CleanupInterval is how often expired challenges are cleaned up
	CleanupInterval = 1 * time.Minute

	// TokenBytes is the number of random bytes for tokens
	TokenBytes = 32
)

var (
	ErrChallengeNotFound  = errors.New("challenge not found or expired")
	ErrTooManyChallenges  = errors.New("too many pending challenges")
	ErrInvalidPollSecret  = errors.New("invalid poll secret")
)

// challengeEntry stores a pending authentication challenge
type challengeEntry struct {
	ExpiresAt  time.Time
	PollSecret string // Secret required to poll for results (not shown to user)
	// These are set when the challenge is completed
	Completed bool
	Token     string
	UserID    string
}

// ChallengeStore manages pending authentication challenges
type ChallengeStore struct {
	mu         sync.RWMutex
	challenges map[string]*challengeEntry
	stopCh     chan struct{}
}

var store *ChallengeStore

// Init initializes the auth package
func Init() {
	store = &ChallengeStore{
		challenges: make(map[string]*challengeEntry),
		stopCh:     make(chan struct{}),
	}
	go store.cleanupLoop()
}

// cleanupLoop periodically removes expired challenges
func (s *ChallengeStore) cleanupLoop() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

// cleanup removes expired challenges
func (s *ChallengeStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for challenge, entry := range s.challenges {
		if now.After(entry.ExpiresAt) {
			delete(s.challenges, challenge)
		}
	}
}

// GenerateChallenge creates a new authentication challenge
// Returns: challenge (public, sent to Matrix), pollSecret (private, for polling), expiresAt, error
func GenerateChallenge(ctx context.Context) (string, string, time.Time, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	// Check if we have too many pending challenges
	if len(store.challenges) >= MaxPendingChallenges {
		log.Warn().Ctx(ctx).Int("count", len(store.challenges)).Msg("Too many pending auth challenges")
		return "", "", time.Time{}, ErrTooManyChallenges
	}

	// Generate random challenge (public - shown to user, sent on Matrix)
	challenge, err := generateRandomString(TokenBytes)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate auth challenge")
		return "", "", time.Time{}, err
	}

	// Generate poll secret (private - only the originating browser knows this)
	pollSecret, err := generateRandomString(TokenBytes)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate poll secret")
		return "", "", time.Time{}, err
	}

	expiresAt := time.Now().Add(ChallengeTTL)
	store.challenges[challenge] = &challengeEntry{
		ExpiresAt:  expiresAt,
		PollSecret: pollSecret,
	}

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Time("expires_at", expiresAt).Msg("Generated auth challenge")
	return challenge, pollSecret, expiresAt, nil
}

// CompleteChallenge marks a challenge as completed with the authenticated user
func CompleteChallenge(ctx context.Context, challenge, userID, token string) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	entry, exists := store.challenges[challenge]
	if !exists {
		log.Debug().Ctx(ctx).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Challenge not found")
		return ErrChallengeNotFound
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(store.challenges, challenge)
		log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Msg("Challenge expired")
		return ErrChallengeNotFound
	}

	entry.Completed = true
	entry.Token = token
	entry.UserID = userID

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Str("user_id", userID).Msg("Challenge completed")
	return nil
}

// PollChallenge checks if a challenge has been completed
// Requires the poll secret to prevent attackers from stealing the token
// Returns (completed, token, userID, error)
func PollChallenge(ctx context.Context, challenge, pollSecret string) (bool, string, string, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	entry, exists := store.challenges[challenge]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return false, "", "", ErrChallengeNotFound
	}

	// Verify poll secret
	if entry.PollSecret != pollSecret {
		log.Warn().Ctx(ctx).Str("challenge", challenge[:8]+"...").Msg("Invalid poll secret attempted")
		return false, "", "", ErrInvalidPollSecret
	}

	return entry.Completed, entry.Token, entry.UserID, nil
}

// ConsumeChallengeResult retrieves and removes a completed challenge
// Requires the poll secret to prevent attackers from stealing the token
// This should be called when the frontend receives the token
func ConsumeChallengeResult(ctx context.Context, challenge, pollSecret string) (string, string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	entry, exists := store.challenges[challenge]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return "", "", ErrChallengeNotFound
	}

	// Verify poll secret
	if entry.PollSecret != pollSecret {
		log.Warn().Ctx(ctx).Str("challenge", challenge[:8]+"...").Msg("Invalid poll secret attempted on consume")
		return "", "", ErrInvalidPollSecret
	}

	if !entry.Completed {
		return "", "", ErrChallengeNotFound
	}

	token := entry.Token
	userID := entry.UserID
	delete(store.challenges, challenge)

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Str("user_id", userID).Msg("Challenge result consumed")
	return token, userID, nil
}

// GenerateSessionToken creates a new random session token
func GenerateSessionToken() (string, error) {
	return generateRandomString(TokenBytes)
}

// generateRandomString generates a cryptographically random base64url-encoded string
func generateRandomString(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
