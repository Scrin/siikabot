package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// useStore gives a test its own store, on a clock it controls
func useStore(t *testing.T) *time.Time {
	t.Helper()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	previous := store
	store = newStore(func() time.Time { return now })
	t.Cleanup(func() { store = previous })
	return &now
}

// issueToken is an issue function for CompleteChallenge that hands out a fixed token
func issueToken(token string) func() (string, error) {
	return func() (string, error) { return token, nil }
}

func TestChallengeRoundTrip(t *testing.T) {
	useStore(t)
	ctx := context.Background()

	challenge, pollSecret, _, err := GenerateChallenge(ctx)
	if err != nil {
		t.Fatalf("GenerateChallenge() error = %v", err)
	}
	if _, _, err := ConsumeChallengeResult(ctx, challenge, pollSecret); !errors.Is(err, ErrChallengeNotFound) {
		t.Errorf("polling a challenge nobody completed = %v, want it pending", err)
	}
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Fatalf("CompleteChallenge() error = %v", err)
	}
	token, userID, err := ConsumeChallengeResult(ctx, challenge, pollSecret)
	if err != nil || token != "alice-token" || userID != "@alice:example.com" {
		t.Errorf("ConsumeChallengeResult() = %q, %q, %v, want Alice's token", token, userID, err)
	}
	if _, _, err := ConsumeChallengeResult(ctx, challenge, pollSecret); err == nil {
		t.Error("a challenge's result can be picked up twice")
	}
}

// Anyone in the room sees the challenge someone sends with !auth. Completing it again would log the browser
// that asked for it in as someone else.
func TestChallengeCanBeCompletedOnlyOnce(t *testing.T) {
	useStore(t)
	ctx := context.Background()
	challenge, pollSecret, _, _ := GenerateChallenge(ctx)

	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Fatalf("CompleteChallenge() error = %v", err)
	}
	issued := false
	err := CompleteChallenge(ctx, challenge, "@mallory:example.com", func() (string, error) {
		issued = true
		return "mallory-token", nil
	})
	if !errors.Is(err, ErrChallengeCompleted) || issued {
		t.Errorf("completing a challenge again = %v, issued a token %v, want it refused first", err, issued)
	}
	if _, userID, _ := ConsumeChallengeResult(ctx, challenge, pollSecret); userID != "@alice:example.com" {
		t.Errorf("the browser is logged in as %q, want Alice", userID)
	}
}

// A mistyped or expired challenge must not touch the session the sender already has
func TestChallengeThatDoesntCheckOutIssuesNoToken(t *testing.T) {
	now := useStore(t)
	ctx := context.Background()
	challenge, _, _, _ := GenerateChallenge(ctx)
	other := newStore(time.Now)
	forged, _, _ := other.newChallenge(now.Add(ChallengeTTL))

	for name, test := range map[string]struct {
		challenge string
		later     time.Duration
	}{
		"typo":      {typo(challenge, 10), 0},
		"last_char": {lastCharSpelledOtherwise(t, challenge), 0},
		"garbage":   {"!auth", 0},
		"empty":     {"", 0},
		"forged":    {forged, 0},
		"expired":   {challenge, ChallengeTTL + time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			*now = now.Add(test.later)
			issued := false
			err := CompleteChallenge(ctx, test.challenge, "@alice:example.com", func() (string, error) {
				issued = true
				return "token", nil
			})
			if !errors.Is(err, ErrChallengeNotFound) || issued {
				t.Errorf("CompleteChallenge() = %v, issued a token %v, want it refused first", err, issued)
			}
		})
	}
}

func TestPollNeedsThePollSecret(t *testing.T) {
	useStore(t)
	ctx := context.Background()
	challenge, pollSecret, _, _ := GenerateChallenge(ctx)
	otherChallenge, otherSecret, _, _ := GenerateChallenge(ctx)
	_ = CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token"))

	for _, secret := range []string{"", "x", otherSecret, strings.ToUpper(pollSecret), challenge} {
		if token, _, err := ConsumeChallengeResult(ctx, challenge, secret); !errors.Is(err, ErrInvalidPollSecret) {
			t.Errorf("polling with %q = %q, %v, want the secret refused", secret, token, err)
		}
	}
	if _, _, err := ConsumeChallengeResult(ctx, otherChallenge, pollSecret); !errors.Is(err, ErrInvalidPollSecret) {
		t.Errorf("one challenge's secret works for another: %v", err)
	}
	if token, _, err := ConsumeChallengeResult(ctx, challenge, pollSecret); err != nil || token != "alice-token" {
		t.Errorf("the right secret gives %q, %v", token, err)
	}
}

// Challenges aren't stored, so asking for any number of them fills nothing up and blocks nobody's login
func TestChallengesAreNotStored(t *testing.T) {
	useStore(t)
	ctx := context.Background()
	for range 20000 {
		if _, _, _, err := GenerateChallenge(ctx); err != nil {
			t.Fatalf("GenerateChallenge() error = %v", err)
		}
	}
	if n := len(store.completed); n != 0 {
		t.Errorf("%d challenges were stored", n)
	}
	challenge, _, _, err := GenerateChallenge(ctx)
	if err != nil {
		t.Fatalf("GenerateChallenge() error = %v", err)
	}
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("t")); err != nil {
		t.Errorf("CompleteChallenge() after many challenges error = %v", err)
	}
}

// The browser can pick the result up a little after the challenge has expired, if it was completed in time
func TestCompletedChallengeWaitsForItsBrowser(t *testing.T) {
	now := useStore(t)
	ctx := context.Background()
	challenge, pollSecret, _, _ := GenerateChallenge(ctx)

	*now = now.Add(ChallengeTTL - time.Second)
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Fatalf("CompleteChallenge() error = %v", err)
	}
	*now = now.Add(2 * time.Second)
	if token, _, err := ConsumeChallengeResult(ctx, challenge, pollSecret); err != nil || token != "alice-token" {
		t.Errorf("ConsumeChallengeResult() just after the challenge expired = %q, %v", token, err)
	}
}

// A failure to make the token leaves the challenge as it was, to be tried again
func TestChallengeStaysOpenWhenTheTokenCantBeIssued(t *testing.T) {
	useStore(t)
	ctx := context.Background()
	challenge, pollSecret, _, _ := GenerateChallenge(ctx)

	failure := errors.New("database down")
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", func() (string, error) { return "", failure }); !errors.Is(err, failure) {
		t.Errorf("CompleteChallenge() = %v, want the issue error", err)
	}
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Errorf("CompleteChallenge() after a failure = %v", err)
	}
	if token, _, _ := ConsumeChallengeResult(ctx, challenge, pollSecret); token != "alice-token" {
		t.Errorf("token = %q, want alice-token", token)
	}
}

// typo changes one character of a challenge into another
func typo(challenge string, i int) string {
	replacement := byte('a')
	if challenge[i] == 'a' {
		replacement = 'b'
	}
	return challenge[:i] + string(replacement) + challenge[i+1:]
}

// lastCharSpelledOtherwise spells a challenge's last character another way that a lenient decoder reads as the
// same bytes: its last two bits are unused
func lastCharSpelledOtherwise(t *testing.T, challenge string) string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, challenge[len(challenge)-1])
	if last%4 != 0 {
		t.Fatalf("the challenge's last character %q has its unused bits set", challenge[len(challenge)-1])
	}
	return challenge[:len(challenge)-1] + string(alphabet[last+1])
}

// A result whose browser never came is forgotten once it expires, but never while its token is being made
func TestCleanupForgetsResultsNobodyCameFor(t *testing.T) {
	now := useStore(t)
	ctx := context.Background()
	abandoned, _, _, _ := GenerateChallenge(ctx)
	if err := CompleteChallenge(ctx, abandoned, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(50 * time.Second)
	waiting, pollSecret, _, _ := GenerateChallenge(ctx)
	if err := CompleteChallenge(ctx, waiting, "@bob:example.com", issueToken("bob-token")); err != nil {
		t.Fatal(err)
	}
	store.completed["being-made"] = &completion{pending: true}
	*now = now.Add(20 * time.Second)

	store.mu.Lock()
	store.cleanup()
	_, abandonedKept := store.completed[abandoned]
	_, pendingKept := store.completed["being-made"]
	store.mu.Unlock()
	if abandonedKept {
		t.Error("a result past its time is still kept")
	}
	if !pendingKept {
		t.Error("a result whose token is being made was forgotten")
	}
	if token, _, err := ConsumeChallengeResult(ctx, waiting, pollSecret); err != nil || token != "bob-token" {
		t.Errorf("a result still in its time = %q, %v", token, err)
	}
}

// A full store makes room by forgetting expired results, and refuses more only when none have expired
func TestFullStoreMakesRoom(t *testing.T) {
	now := useStore(t)
	ctx := context.Background()
	for i := range MaxCompletedChallenges {
		store.completed[fmt.Sprint("old-", i)] = &completion{token: "old", expiresAt: now.Add(time.Second)}
	}

	challenge, _, _, _ := GenerateChallenge(ctx)
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); !errors.Is(err, ErrTooManyChallenges) {
		t.Errorf("completing a challenge in a store full of live results = %v, want ErrTooManyChallenges", err)
	}
	*now = now.Add(2 * time.Second)
	challenge, _, _, _ = GenerateChallenge(ctx)
	if err := CompleteChallenge(ctx, challenge, "@alice:example.com", issueToken("alice-token")); err != nil {
		t.Errorf("completing a challenge once the full store's results expired = %v", err)
	}
	if len(store.completed) != 1 {
		t.Errorf("the store holds %d results, want only the new one", len(store.completed))
	}
}
