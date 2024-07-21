package database

import (
	"database/sql"
	"errors"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	_ "github.com/mattn/go-sqlite3"
)

type DBConn struct {
	db  *sql.DB
	mux sync.Mutex
}

// info_hash => stored as hex
// torrent_file => bEncoded dict of the torrent metadata
// path => path to download target
// status => 0 (paused) / 1 (running)

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
		uploadrate_max INTEGER,
    status INTEGER
    );` // status: 0 paused, 1 running
	if _, err := db.Exec(createTorrents); err != nil {
		return nil, err
	}
	return &DBConn{
		db: db,
	}, nil
}

func (dbc *DBConn) Disconnect() {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()
	dbc.db.Close()
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
	bf, err := bitfield.FromHex(bitfieldHex, bitfieldLength)
	if err != nil {
		return nil, err
	}
	return bf, nil
}

func (dbc *DBConn) UpdateBitfield(infohash []byte, bf *bitfield.Bitfield) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infohash)
	bitfieldHex := bf.ToHex()

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

func (dbc *DBConn) AddTorrent(tf *torrentfile.TorrentFile, bf *bitfield.Bitfield, path string, downloadrate_max int, uploadrate_max int, status int) error {
	dbc.mux.Lock()
	dbc.mux.Unlock()

	infoHashStr := string(tf.InfoHash)
	torrentFileStr, err := tf.Bencode()
	if err != nil {
		return err
	}
	// path
	bitfieldHex := bf.ToHex()
	bitfieldLength := bf.Len()
	//dl_max
	//up_max
	//status
	//
	row := dbc.db.QueryRow("SELECT info_hash FROM torrents WHERE info_hash = ?", infoHashStr)
	var existingInfoHash string
	err = row.Scan(&existingInfoHash)
	if err != sql.ErrNoRows {
		return errors.New("torrent already exists")
	}

	const query = `
    INSERT INTO torrents (info_hash, torrent_file, path, bitfield_hex, bitfield_length, downloadrate_max, uploadrate_max, status)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `

	_, err = dbc.db.Exec(
		query,
		infoHashStr,
		torrentFileStr,
		path,
		bitfieldHex,
		bitfieldLength,
		downloadrate_max,
		uploadrate_max,
		status,
	)
	if err != nil {
		return err
	}

	return nil
}

func (dbc *DBConn) DeleteTorrent(infoHash []byte) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)
	row := dbc.db.QueryRow("SELECT info_hash FROM torrents WHERE info_hash = ?", infoHashStr)
	var existingInfoHash string
	err := row.Scan(&existingInfoHash)
	if err != nil {
		return err
	}

	const query = `
    DELETE FROM torrents WHERE info_hash = ?
    `
	_, err = dbc.db.Exec(query, infoHashStr)
	if err != nil {
		return err
	}
	return nil
}

func (dbc *DBConn) UpdateTorrentStatus(infoHash []byte, status int) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
	UPDATE torrents
	SET status = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(query, status, infoHashStr)
	if err != nil {
		return err
	}
	return nil
}
