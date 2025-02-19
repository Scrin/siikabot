package db

import (
	"database/sql"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

var (
	db   *sql.DB
	lock sync.RWMutex
)

func Set(k, v string) {
	lock.Lock()
	defer lock.Unlock()

	stmt, err := db.Prepare("replace into kv(k, v) values(?, ?)")
	if err != nil {
		log.Error().Err(err).Str("key", k).Msg("Failed to prepare set statement")
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(k, v)
	if err != nil {
		log.Error().Err(err).Str("key", k).Msg("Failed to execute set statement")
	}
}

func Get(k string) string {
	lock.RLock()
	defer lock.RUnlock()

	stmt, err := db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Error().Err(err).Str("key", k).Msg("Failed to prepare get statement")
		return ""
	}
	defer stmt.Close()
	var resp string
	err = stmt.QueryRow(k).Scan(&resp)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Str("key", k).Msg("Failed to execute get statement")
	}
	return resp
}

func Init(dbFile string) error {
	var err error
	lock.Lock()
	defer lock.Unlock()

	if db, err = sql.Open("sqlite3", dbFile); err != nil {
		log.Error().Err(err).Str("db_file", dbFile).Msg("Failed to open database")
		return err
	}
	if _, err := db.Exec("create table if not exists kv (k text not null primary key, v text);"); err != nil {
		log.Error().Err(err).Str("db_file", dbFile).Msg("Failed to create table")
		return err
	}
	return nil
}
