// Package dbtest gives a test a database of its own, with the bot's migrations run, and makes the db package work
// on it while the test runs (see pgtest for where the database is). The db package works on one database at a time,
// so tests that use this can't run in parallel.
//
// A package whose tests use it calls Main, or pgtest.Run, from TestMain.
//
// The db package's own tests can't import this, which imports db, so they do the same with pgtest directly.
package dbtest

import (
	"context"
	"testing"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/testutil/pgtest"
)

// Main runs a package's tests, then stops the database cluster they started, if they started one
func Main(m *testing.M) {
	pgtest.Main(m)
}

// Use gives t a new database with the bot's migrations run, makes the db package work on it until t ends, and
// returns its connection string, for reading it directly
func Use(t testing.TB) string {
	t.Helper()
	connString := pgtest.Prepared(t, "migrated", db.Migrate)
	restore, err := db.Use(context.Background(), connString)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(restore)
	return connString
}
