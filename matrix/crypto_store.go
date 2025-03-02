package matrix

import (
	"context"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/crypto"
)

type CryptoStore struct {
	*crypto.SQLCryptoStore
}

func NewCryptoStore() *CryptoStore {
	database, err := dbutil.NewWithDB(db.GetDB(), "postgres")
	if err != nil {
		log.Error().Err(err).Msg("Failed to create crypto store")
		return nil
	}
	cs := crypto.NewSQLCryptoStore(database, &dbLogger{}, config.UserID, "siikabot", []byte(config.PickleKey))
	cs.DB.Upgrade(context.Background())
	return &CryptoStore{SQLCryptoStore: cs}
}

type dbLogger struct{}

// DoUpgrade implements dbutil.DatabaseLogger.
func (dbLogger) DoUpgrade(from int, to int, message string, txn dbutil.TxnMode) {
	log.Info().
		Int("from", from).
		Int("to", to).
		Str("message", message).
		Str("txn", string(txn)).
		Msg("Database upgrade")
}

// PrepareUpgrade implements dbutil.DatabaseLogger.
func (dbLogger) PrepareUpgrade(current int, compat int, latest int) {
	log.Info().
		Int("current", current).
		Int("compat", compat).
		Int("latest", latest).
		Msg("Database upgrade")
}

// QueryTiming implements dbutil.DatabaseLogger.
func (dbLogger) QueryTiming(ctx context.Context, method string, query string, args []any, nrows int, duration time.Duration, err error) {
	log.Info().
		Err(err).
		Str("method", method).
		Str("query", query).
		Interface("args", args).
		Int("nrows", nrows).
		Dur("duration", duration).
		Msg("Database query")
}

// Warn implements dbutil.DatabaseLogger.
func (dbLogger) Warn(msg string, args ...any) {
	log.Warn().
		Str("msg", msg).
		Interface("args", args).
		Msg("Database warning")
}

// WarnUnsupportedVersion implements dbutil.DatabaseLogger.
func (dbLogger) WarnUnsupportedVersion(current int, compat int, latest int) {
	log.Warn().
		Int("current", current).
		Int("compat", compat).
		Int("latest", latest).
		Msg("Database warning")
}
