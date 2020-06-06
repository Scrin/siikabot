package db

import (
	"database/sql"
	"log"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db   *sql.DB
	lock sync.RWMutex
}

func (db *DB) Set(k, v string) {
	db.lock.Lock()
	defer db.lock.Unlock()

	stmt, err := db.db.Prepare("replace into kv(k, v) values(?, ?)")
	if err != nil {
		log.Print(err)
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(k, v)
	if err != nil {
		log.Print(err)
	}
}

func (db *DB) Get(k string) string {
	db.lock.RLock()
	defer db.lock.RUnlock()

	stmt, err := db.db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Print(err)
		return ""
	}
	defer stmt.Close()
	var resp string
	err = stmt.QueryRow(k).Scan(&resp)
	if err != nil {
		log.Print(err)
	}
	return resp
}

func NewDB(dbFile string) *DB {
	db := DB{}
	db.lock.Lock()
	defer db.lock.Unlock()
	var err error

	if db.db, err = sql.Open("sqlite3", dbFile); err != nil {
		log.Fatal(err)
	}
	if _, err := db.db.Exec("create table if not exists kv (k text not null primary key, v text);"); err != nil {
		log.Fatal(err)
	}
	return &db
}
