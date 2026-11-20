package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The database holds only hashes of the tokens, so reading it logs nobody in
func TestSessionTokensAreStoredHashed(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	if err := SetWebSessionToken(ctx, "@alice:example.com", "alice-token"); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT web_session_token_hash FROM user_authorizations WHERE user_id = '@alice:example.com'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == "alice-token" || stored != hashSessionToken("alice-token") {
		t.Errorf("the stored token is %q, want its hash", stored)
	}

	if user, err := GetUserByWebSessionToken(ctx, "alice-token"); err != nil || user.UserID != "@alice:example.com" {
		t.Errorf("GetUserByWebSessionToken() = %+v, %v, want Alice", user, err)
	}
	for _, token := range []string{"", "alice", stored} {
		if _, err := GetUserByWebSessionToken(ctx, token); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("GetUserByWebSessionToken(%q) error = %v, want it invalid", token, err)
		}
	}
}

// A new session ends the one before it, and logging out ends it
func TestSessionTokenReplacementAndLogout(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()

	_ = SetWebSessionToken(ctx, "@alice:example.com", "first")
	_ = SetWebSessionToken(ctx, "@bob:example.com", "bobs")
	_ = SetWebSessionToken(ctx, "@alice:example.com", "second")
	if _, err := GetUserByWebSessionToken(ctx, "first"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("the replaced token still works: %v", err)
	}
	if _, err := GetUserByWebSessionToken(ctx, "second"); err != nil {
		t.Errorf("the new token doesn't work: %v", err)
	}

	if err := ClearWebSessionToken(ctx, "@alice:example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetUserByWebSessionToken(ctx, "second"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("the token works after logging out: %v", err)
	}
	if user, err := GetUserByWebSessionToken(ctx, "bobs"); err != nil || user.UserID != "@bob:example.com" {
		t.Errorf("Alice logging out ended Bob's session: %+v, %v", user, err)
	}
}

// A session ends once it has gone unused for SessionIdleTimeout, and using it keeps it going
func TestSessionEndsWhenUnused(t *testing.T) {
	useTestDatabase(t)
	ctx := context.Background()
	_ = SetWebSessionToken(ctx, "@alice:example.com", "alice-token")

	lastUsed := func(ago time.Duration) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE user_authorizations SET web_session_last_used_at = now() - make_interval(secs => $1)`,
			ago.Seconds()); err != nil {
			t.Fatal(err)
		}
	}

	lastUsed(SessionIdleTimeout - 2*time.Hour)
	if _, err := GetUserByWebSessionToken(ctx, "alice-token"); err != nil {
		t.Fatalf("a session used just within the timeout has ended: %v", err)
	}
	var idle time.Duration
	var seconds float64
	if err := pool.QueryRow(ctx, `SELECT extract(epoch FROM now() - web_session_last_used_at) FROM user_authorizations`).Scan(&seconds); err != nil {
		t.Fatal(err)
	}
	if idle = time.Duration(seconds * float64(time.Second)); idle > time.Minute {
		t.Errorf("using the session didn't record it as used: last used %v ago", idle)
	}

	lastUsed(SessionIdleTimeout + time.Minute)
	if _, err := GetUserByWebSessionToken(ctx, "alice-token"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("a session unused for longer than the timeout still works: %v", err)
	}
}

// The migration that started hashing the tokens keeps the sessions people had
func TestHashingMigrationKeepsSessions(t *testing.T) {
	const migration = "0033__hash-web-session-tokens.sql"
	p := useEmptyTestDatabase(t)
	ctx := context.Background()
	if err := runMigrations(ctx, p, migrationsBefore(migration)); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO user_authorizations (user_id, web_session_token) VALUES
		('@alice:example.com', 'Alice_Token-1'), ('@bob:example.com', NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("running the migration: %v", err)
	}

	if user, err := GetUserByWebSessionToken(ctx, "Alice_Token-1"); err != nil || user.UserID != "@alice:example.com" {
		t.Errorf("Alice's session after the migration: %+v, %v", user, err)
	}
	var hashes int
	if err := pool.QueryRow(ctx, `SELECT count(web_session_token_hash) FROM user_authorizations`).Scan(&hashes); err != nil {
		t.Fatal(err)
	}
	if hashes != 1 {
		t.Errorf("%d sessions after the migration, want Alice's only", hashes)
	}
}
