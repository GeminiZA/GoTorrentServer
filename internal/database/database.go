package database

import (
	"database/sql"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type DBConn struct {
	db *sql.DB
	mux sync.Mutex
}

func Connect() (*DBConn, error) {
	const file string = "gotorrentserver.db"
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}
	const createTorrents string = `
	CREATE TABLE IF NOT EXISTS torrents (
		info_hash_hex STRING NOT NULL PRIMARY KEY,
		torrent_file STRING NOT NULL,
		path STRING,
		bitfield_hex STRING NOT NULL,
		bitfield_length INTEGER NOT NULL
	);`
	if _, err := db.Exec(createTorrents); err != nil {
		return nil, err
	}
	return &DBConn{
		db: db,
	}, nil
}


func (dbc *DBConn) Disconnect() error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	return dbc.db.Close()
}