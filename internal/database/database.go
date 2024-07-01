package database

import (
	"database/sql"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
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
		info_hash STRING NOT NULL PRIMARY KEY,
		torrent_file STRING NOT NULL,
		path STRING,
		bitfield_hex STRING NOT NULL,
		bitfield_length INTEGER NOT NULL,
		downloadrate_max INTEGER,
		updaterate_max INTEGER
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

func (dbc *DBConn) GetBitfield(infoHash []byte) (*bitfield.Bitfield, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()
	infoHashStr := string(infoHash)

	const query = `
	SELECT bitfield_hex, bitfield_length
	FROM torrents
	WHERE info_hash = ?
	`

	var bitfieldLength int64
	var bitfieldHex string
	err := dbc.db.QueryRow(query, infoHashStr).Scan(&bitfieldHex, &bitfieldLength)
	if err != nil {
		return nil, err
	}
	bitfield, err  := bitfield.FromHex(bitfieldHex, bitfieldLength)
	if err != nil {
		return nil, err
	}
	return bitfield, nil
}

func (dbc *DBConn) UpdateBitfield(infohash []byte, bitfield *bitfield.Bitfield) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infohash)
	bitfieldHex := bitfield.ToHex()

	const query = `
	UPDATE torrents
	SET bitfield_hex = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(query, bitfieldHex, infoHashStr)
	if err != nil {
		return err
	}
	return nil
}