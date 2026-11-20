package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// ChallengeTTL is how long a challenge is valid, and how long a completed one waits for its browser
	ChallengeTTL = 1 * time.Minute

	// MaxCompletedChallenges is the most completed challenges that wait for their browsers at once. Only
	// !auth completes one, so it takes that many messages in a minute to reach this.
	MaxCompletedChallenges = 1000

	// CleanupInterval is how often completed challenges whose browsers never came are forgotten
	CleanupInterval = 1 * time.Minute

	// TokenBytes is the number of random bytes for tokens
	TokenBytes = 32

	// nonceBytes and macBytes are the sizes of the parts of a challenge around its expiry time
	nonceBytes = 16
	macBytes   = 16
)

var (
	ErrChallengeNotFound  = errors.New("challenge not found or expired")
	ErrChallengeCompleted = errors.New("challenge already completed")
	ErrTooManyChallenges  = errors.New("too many completed challenges waiting")
	ErrInvalidPollSecret  = errors.New("invalid poll secret")
)

// A challenge is signed rather than stored. It holds a random nonce and when it expires, with a MAC over them
// made with a key the bot picks at startup, so the bot can check a challenge it made without keeping it, and
// anyone may ask for as many as they like without filling anything up. The poll secret for a challenge is a MAC
// of the challenge with the same key: only the browser that asked for the challenge got it, and only the bot
// can make it.
//
// Only completing a challenge stores anything, until its browser picks the result up.

// completion is a challenge that someone completed with !auth
type completion struct {
	expiresAt time.Time
	// pending is set while the session token is being made and stored
	pending bool
	userID  string
	token   string
}

// ChallengeStore holds the key challenges are signed with, and the completed challenges
type ChallengeStore struct {
	key       []byte
	mu        sync.Mutex
	completed map[string]*completion
	now       func() time.Time
	stopCh    chan struct{}
}

var store *ChallengeStore

// Init initializes the auth package
func Init() {
	store = newStore(time.Now)
	go store.cleanupLoop()
}

// newStore makes a store with a new random key
func newStore(now func() time.Time) *ChallengeStore {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("auth: no randomness for the challenge key: " + err.Error())
	}
	return &ChallengeStore{
		key:       key,
		completed: make(map[string]*completion),
		now:       now,
		stopCh:    make(chan struct{}),
	}
}

// cleanupLoop periodically forgets completed challenges whose browsers never came
func (s *ChallengeStore) cleanupLoop() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			s.cleanup()
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}

// cleanup forgets completed challenges whose browsers never came. The caller holds the lock.
func (s *ChallengeStore) cleanup() {
	now := s.now()
	for challenge, entry := range s.completed {
		if !entry.pending && now.After(entry.expiresAt) {
			delete(s.completed, challenge)
		}
	}
}

// mac signs data with the store's key, under a label that keeps a challenge's MAC from ever being a poll
// secret
func (s *ChallengeStore) mac(label string, data []byte) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(label))
	h.Write([]byte{0})
	h.Write(data)
	return h.Sum(nil)
}

// challengeEncoding is how challenges are written. Decoding is strict, so a challenge has only one spelling:
// otherwise the unused bits of its last character could spell it another way, which would count as another
// challenge in the completed ones.
var challengeEncoding = base64.RawURLEncoding.Strict()

// newChallenge makes a challenge that expires at expiresAt, and its poll secret. The nonce comes first, so
// that the start of a challenge, which the logs show, tells challenges apart.
func (s *ChallengeStore) newChallenge(expiresAt time.Time) (string, string, error) {
	payload := make([]byte, nonceBytes+8)
	if _, err := rand.Read(payload[:nonceBytes]); err != nil {
		return "", "", err
	}
	binary.BigEndian.PutUint64(payload[nonceBytes:], uint64(expiresAt.Unix()))
	challenge := challengeEncoding.EncodeToString(append(payload, s.mac("challenge", payload)[:macBytes]...))
	return challenge, s.pollSecret(challenge), nil
}

// pollSecret is the poll secret of a challenge
func (s *ChallengeStore) pollSecret(challenge string) string {
	return base64.RawURLEncoding.EncodeToString(s.mac("poll", []byte(challenge)))
}

// verify checks that the bot made a challenge, and returns when it expires
func (s *ChallengeStore) verify(challenge string) (time.Time, error) {
	raw, err := challengeEncoding.DecodeString(challenge)
	if err != nil || len(raw) != nonceBytes+8+macBytes {
		return time.Time{}, ErrChallengeNotFound
	}
	payload, mac := raw[:nonceBytes+8], raw[nonceBytes+8:]
	if !hmac.Equal(mac, s.mac("challenge", payload)[:macBytes]) {
		return time.Time{}, ErrChallengeNotFound
	}
	return time.Unix(int64(binary.BigEndian.Uint64(payload[nonceBytes:])), 0), nil
}

// GenerateChallenge creates a new authentication challenge
// Returns: challenge (public, sent to Matrix), pollSecret (private, for polling), expiresAt, error
func GenerateChallenge(ctx context.Context) (string, string, time.Time, error) {
	expiresAt := store.now().Add(ChallengeTTL).Truncate(time.Second)
	challenge, pollSecret, err := store.newChallenge(expiresAt)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate auth challenge")
		return "", "", time.Time{}, err
	}

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Time("expires_at", expiresAt).Msg("Generated auth challenge")
	return challenge, pollSecret, expiresAt, nil
}

// CompleteChallenge completes a challenge for a user, with the session token issue makes, which the challenge's
// browser then picks up. A challenge can be completed only once: anyone in a room where someone sends !auth sees
// the challenge, and completing it again would log that person's browser in as someone else. The token is made
// only once the challenge has been checked.
func CompleteChallenge(ctx context.Context, challenge, userID string, issue func() (string, error)) error {
	expiresAt, err := store.verify(challenge)
	if err != nil || store.now().After(expiresAt) {
		log.Debug().Ctx(ctx).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Challenge not found or expired")
		return ErrChallengeNotFound
	}

	store.mu.Lock()
	if _, exists := store.completed[challenge]; exists {
		store.mu.Unlock()
		log.Warn().Ctx(ctx).Str("challenge", challenge[:8]+"...").Str("user_id", userID).Msg("Challenge completed again")
		return ErrChallengeCompleted
	}
	if len(store.completed) >= MaxCompletedChallenges {
		store.cleanup()
	}
	if len(store.completed) >= MaxCompletedChallenges {
		store.mu.Unlock()
		log.Warn().Ctx(ctx).Int("count", len(store.completed)).Msg("Too many completed auth challenges waiting")
		return ErrTooManyChallenges
	}
	entry := &completion{pending: true}
	store.completed[challenge] = entry
	store.mu.Unlock()

	token, err := issue()

	store.mu.Lock()
	defer store.mu.Unlock()
	if err != nil {
		delete(store.completed, challenge)
		return err
	}
	entry.pending = false
	entry.userID = userID
	entry.token = token
	// Its browser may come for it a little after the challenge itself has expired
	entry.expiresAt = store.now().Add(ChallengeTTL)

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Str("user_id", userID).Msg("Challenge completed")
	return nil
}

// ConsumeChallengeResult retrieves and removes a completed challenge. It requires the challenge's poll secret, so
// that only the browser that asked for the challenge gets the token. Until the challenge is completed, it returns
// ErrChallengeNotFound.
func ConsumeChallengeResult(ctx context.Context, challenge, pollSecret string) (string, string, error) {
	if subtle.ConstantTimeCompare([]byte(pollSecret), []byte(store.pollSecret(challenge))) != 1 {
		log.Warn().Ctx(ctx).Str("challenge", challenge[:min(8, len(challenge))]+"...").Msg("Invalid poll secret attempted")
		return "", "", ErrInvalidPollSecret
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	entry, exists := store.completed[challenge]
	if !exists || entry.pending || store.now().After(entry.expiresAt) {
		return "", "", ErrChallengeNotFound
	}
	delete(store.completed, challenge)

	log.Debug().Ctx(ctx).Str("challenge", challenge[:8]+"...").Str("user_id", entry.userID).Msg("Challenge result consumed")
	return entry.token, entry.userID, nil
}

// GenerateSessionToken creates a new random session token
func GenerateSessionToken() (string, error) {
	return RandomString(TokenBytes)
}

// RandomString generates a cryptographically random base64url-encoded string from the given number
// of random bytes
func RandomString(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
