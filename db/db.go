package db

import (
	"database/sql"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog/log"
)

var (
	db *sql.DB
)

func Init() error {
	err := setupPostgres(config.PostgresConnectionString)
	if err != nil {
		log.Error().Err(err).Msg("Failed to setup postgres")
		return err
	}
	return nil
}
