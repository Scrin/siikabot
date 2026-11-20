package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testPostgresEnv names the connection string of the PostgreSQL server the database tests run against. Its user
// has to be able to create databases. Without it, the tests that need a database are skipped.
const testPostgresEnv = "SIIKABOT_TEST_POSTGRES"

// useTestDatabase gives a test a database of its own as the package's pool, with the migrations include picks
// run, and drops it after
func useTestDatabase(t *testing.T, include func(fileName string) bool) {
	t.Helper()
	dsn := os.Getenv(testPostgresEnv)
	if dsn == "" {
		t.Skip(testPostgresEnv + " isn't set")
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to %s: %v", testPostgresEnv, err)
	}
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	name := "siikabot_test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close(ctx)
		t.Fatalf("creating the test database: %v", err)
	}

	conf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conf.ConnConfig.Database = name
	testPool, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		t.Fatal(err)
	}

	previous := pool
	pool = testPool
	t.Cleanup(func() {
		pool = previous
		testPool.Close()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping the test database: %v", err)
		}
		admin.Close(ctx)
	})

	if err := runMigrations(ctx, include); err != nil {
		t.Fatalf("running the migrations: %v", err)
	}
}

// allMigrations picks every migration
func allMigrations(string) bool { return true }

// migrationsBefore picks the migrations that come before a file
func migrationsBefore(fileName string) func(string) bool {
	return func(name string) bool { return name < fileName }
}

func TestMigrationsRunOnAnEmptyDatabase(t *testing.T) {
	useTestDatabase(t, allMigrations)

	// Running them again does nothing
	if err := migrate(); err != nil {
		t.Errorf("running the migrations again: %v", err)
	}
}
