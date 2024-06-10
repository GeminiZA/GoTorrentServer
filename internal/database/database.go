package database

import (
	"database/sql"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

type DBConn struct {
	db *sql.DB
	mu sync.Mutex
}

func Connect() (*DBConn, error) {
	const file string = "gotorrentserver.db"
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}
	const createTorrents string = `
	CREATE TABLE IF NOT EXISTS torrents (
		info_hash STRING NOT NULL PRIMARY KEY,
		name STRING NOT NULL,
		time DATETIME NOT NULL,
		tracker_url STRING NOT NULL,
		path STRING NOT NULL,
		description TEXT,
		complete BOOL,
		parts STRING
	);`
	if _, err := db.Exec(createTorrents); err != nil {
		return nil, err
	}
	const createPartHashes string = `
	CREATE TABLE IF NOT EXISTS parts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		info_hash STRING NOT NULL,
		part_index INTEGER NOT NULL,
		part_hash STRING NOT NULL
	)
	`
	if _, err := db.Exec(createPartHashes); err != nil {
		return nil, err
	}
	const createTorrentInfo string = `
	CREATE TABLE IF NOT EXISTS torrentinfo (
		info_hash STRING PRIMARY KEY NOT NULL,
		torrent_file STRING NOT NULL
	)
	`
	if _, err := db.Exec(createTorrentInfo); err != nil {
		return nil, err
	}
	return &DBConn{
		db: db,
	}, nil
}


func (dbc *DBConn) Disconnect() error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	return dbc.db.Close()
}