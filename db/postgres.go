package db

import (
	"context"
	"database/sql"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
)

var pool *pgxpool.Pool

// queryFunc is a function that matches the Query func of a pool, connection, transaction, etc
type queryFunc func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)

func Init() (err error) {
	conf, err := pgxpool.ParseConfig(config.PostgresConnectionString)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse postgres config")
		return
	}

	conf.MaxConns = 8

	pool, err = pgxpool.NewWithConfig(context.Background(), conf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create postgres pool")
		return
	}

	metrics.RegisterPgxpoolStatsCollector(pool)

	return migrate()
}

func GetDB() *sql.DB {
	return stdlib.OpenDBFromPool(pool)
}

// GetPoolStats returns the current pool statistics
// Returns nil if pool is not initialized
func GetPoolStats() *pgxpool.Stat {
	if pool == nil {
		return nil
	}
	return pool.Stat()
}
