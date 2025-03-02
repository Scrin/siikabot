package db

import (
	"database/sql"

	"github.com/Scrin/siikabot/config"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

var (
	db *sql.DB
)

func Init() error {
	err := setupPostgres(config.PostgresConnectionString)
	if err != nil {
		return err
	}

	dbFile := config.DataPath + "/siikabot.db"
	if db, err = sql.Open("sqlite3", dbFile); err != nil {
		log.Error().Err(err).Str("db_file", dbFile).Msg("Failed to open database")
		return err
	}
	if _, err := db.Exec("create table if not exists kv (k text not null primary key, v text);"); err != nil {
		log.Error().Err(err).Str("db_file", dbFile).Msg("Failed to create table")
		return err
	}
	return migrateSQLiteToPostgres()
}
