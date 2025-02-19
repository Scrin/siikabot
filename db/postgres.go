package db

import (
	"context"

	"github.com/Scrin/siikabot/metrics"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var pool *pgxpool.Pool

// queryFunc is a function that matches the Query func of a pool, connection, transaction, etc
type queryFunc func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)

func setupPostgres(connectionString string) (err error) {
	conf, err := pgxpool.ParseConfig(connectionString)
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
