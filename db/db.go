package db

import (
	"database/sql"
	"log"
	"sync"

	_ "github.com/mattn/go-sqlite3"
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
		log.Print(err)
		return
	}
	defer stmt.Close()
	_, err = stmt.Exec(k, v)
	if err != nil {
		log.Print(err)
	}
}

func Get(k string) string {
	lock.RLock()
	defer lock.RUnlock()

	stmt, err := db.Prepare("select v from kv where k = ?")
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

func Init(dbFile string) error {
	var err error
	lock.Lock()
	defer lock.Unlock()

	if db, err = sql.Open("sqlite3", dbFile); err != nil {
		return err
	}
	if _, err := db.Exec("create table if not exists kv (k text not null primary key, v text);"); err != nil {
		return err
	}
	return nil
}
