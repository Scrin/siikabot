package db

import (
	"context"
	"testing"

	"github.com/Scrin/siikabot/testutil/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMain(m *testing.M) {
	pgtest.Main(m)
}

// useTestDatabase gives a test a database of its own with every migration run, and makes the package work on it
// until the test ends. It's dbtest.Use, which this package's tests can't import.
func useTestDatabase(t *testing.T) {
	t.Helper()
	restore, err := Use(context.Background(), pgtest.Prepared(t, "migrated", Migrate))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
}

// useEmptyTestDatabase gives a test an empty database of its own, and makes the package work on it until the
// test ends. It returns the package's pool, for running migrations on.
func useEmptyTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	restore, err := Use(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	return pool
}

// allMigrations picks every migration
func allMigrations(string) bool { return true }

// migrationsBefore picks the migrations that come before a file
func migrationsBefore(fileName string) func(string) bool {
	return func(name string) bool { return name < fileName }
}
