package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

//go:embed migrations
var migrations embed.FS

const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS migrations (
    name TEXT PRIMARY KEY,
    hash TEXT NOT NULL,
    executed_at TIMESTAMP WITH TIME ZONE NOT NULL
);`

func migrate() error {
	ctx := context.Background()

	// Create migrations table if it doesn't exist
	_, err := pool.Exec(ctx, createMigrationsTableSQL)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to create migrations table")
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Read all migration files
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to read migrations directory")
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Get all .sql files and sort them
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Execute each migration file
	for _, fileName := range migrationFiles {
		// Check if migration was already executed
		var exists bool
		var storedHash string
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM migrations WHERE name = $1), COALESCE((SELECT hash FROM migrations WHERE name = $1), '') as hash", fileName).Scan(&exists, &storedHash)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to check migration status")
			return fmt.Errorf("failed to check migration status for %s: %w", fileName, err)
		}

		content, err := fs.ReadFile(migrations, "migrations/"+fileName)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to read migration file")
			return fmt.Errorf("failed to read migration file %s: %w", fileName, err)
		}

		// Calculate hash of the migration content
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		if exists {
			// Verify hash matches
			if hashStr != storedHash {
				log.Error().Ctx(ctx).
					Str("migration_file", fileName).
					Str("stored_hash", storedHash).
					Str("current_hash", hashStr).
					Msg("Migration file has changed after being executed")
				return fmt.Errorf("migration file %s has been modified after execution", fileName)
			}
			log.Debug().Ctx(ctx).Str("migration_file", fileName).Msg("Migration already executed, skipping")
			continue
		}

		// Begin transaction for the migration
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to begin transaction")
			return fmt.Errorf("failed to begin transaction for %s: %w", fileName, err)
		}

		// Execute the migration
		_, err = tx.Exec(ctx, string(content))
		if err != nil {
			tx.Rollback(ctx)
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to execute migration")
			return fmt.Errorf("failed to execute migration %s: %w", fileName, err)
		}

		// Record the migration
		_, err = tx.Exec(ctx, "INSERT INTO migrations (name, hash, executed_at) VALUES ($1, $2, $3)",
			fileName, hashStr, time.Now().UTC())
		if err != nil {
			tx.Rollback(ctx)
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to record migration")
			return fmt.Errorf("failed to record migration %s: %w", fileName, err)
		}

		// Commit the transaction
		err = tx.Commit(ctx)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("migration_file", fileName).Msg("Failed to commit migration transaction")
			return fmt.Errorf("failed to commit migration %s: %w", fileName, err)
		}

		log.Info().Ctx(ctx).Str("migration_file", fileName).Msg("Successfully executed migration")
	}

	return nil
}
