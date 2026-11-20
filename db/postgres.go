package db

import (
	"context"
	"database/sql"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/exaring/otelpgx"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
)

var pool *pgxpool.Pool

// queryFunc is a function that matches the Query func of a pool, connection, transaction, etc
type queryFunc func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)

// Init connects to the database in the configuration, runs its migrations, and reports the connection pool's
// statistics as metrics
func Init() error {
	p, err := connect(context.Background(), config.PostgresConnectionString)
	if err != nil {
		return err
	}
	pool = p
	metrics.RegisterPgxpoolStatsCollector(pool)
	return migrate()
}

// connect makes a connection pool for a database
func connect(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	conf, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to parse postgres config")
		return nil, err
	}

	conf.MaxConns = 8

	// Every query becomes a span, with the SQL as an attribute. Cheap, and it means a failing query
	// shows up in the trace of whatever caused it rather than only in the logs.
	conf.ConnConfig.Tracer = otelpgx.NewTracer()

	p, err := pgxpool.NewWithConfig(ctx, conf)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to create postgres pool")
		return nil, err
	}
	return p, nil
}

// Migrate runs the migrations that haven't run yet on the database at connString, and closes its connections to
// it after. Init runs them on the bot's own database; this prepares another, such as a test's.
func Migrate(ctx context.Context, connString string) error {
	p, err := connect(ctx, connString)
	if err != nil {
		return err
	}
	defer p.Close()
	return runMigrations(ctx, p, func(string) bool { return true })
}

// Use makes the package work on the database at connString, whose migrations have run, until the function it
// returns is called, which goes back to the database it worked on before. The package works on one database at a
// time, so this is for tests, which then can't run in parallel; the bot calls Init.
func Use(ctx context.Context, connString string) (func(), error) {
	p, err := connect(ctx, connString)
	if err != nil {
		return nil, err
	}
	previous := pool
	pool = p
	return func() {
		pool = previous
		p.Close()
	}, nil
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
