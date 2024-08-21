package database

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	_ "github.com/mattn/go-sqlite3"
)

type DBConn struct {
	db  *sql.DB
	mux sync.Mutex

	logger *logger.Logger
}

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
		bitfield BLOB NOT NULL,
		bitfield_length INTEGER NOT NULL,
		downloadrate_max REAL,
		uploadrate_max REAL,
    status INTEGER
    );` // status: 0 paused, 1 running
	if _, err := db.Exec(createTorrents); err != nil {
		return nil, err
	}
	return &DBConn{
		db:     db,
		logger: logger.New(logger.DEBUG, "Database"),
	}, nil
}

func (dbc *DBConn) Disconnect() {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()
	dbc.db.Close()
}

func (dbc *DBConn) GetAllInfoHashes() ([][]byte, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	const query = `
    SELECT info_hash
    FROM torrents
    `

	infoHashes := make([][]byte, 0)
	rows, err := dbc.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var infoHashStr string
		err = rows.Scan(&infoHashStr)
		if err != nil {
			return nil, err
		}
		infoHashes = append(infoHashes, []byte(infoHashStr))
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return infoHashes, nil
}

func (dbc *DBConn) RemoveTorrent(infoHash []byte) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    DELETE FROM torrents WHERE info_hash = ?
    `

	_, err := dbc.db.Exec(query, infoHashStr)
	if err != nil {
		return err
	}

	return nil
}

func (dbc *DBConn) GetTorrentFile(infoHash []byte) (string, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    SELECT torrent_file
    FROM torrents
    WHERE info_hash = ?
    `
	var torrentFileStr string

	err := dbc.db.QueryRow(query, infoHashStr).Scan(&torrentFileStr)
	if err != nil {
		return "", err
	}

	return torrentFileStr, nil
}

func (dbc *DBConn) GetDownloadRate(infoHash []byte) (float64, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    SELECT downloadrate_max
    FROM torrents
    WHERE info_hash = ?
    `
	var downloadrate float64

	err := dbc.db.QueryRow(query, infoHashStr).Scan(&downloadrate)
	if err != nil {
		return 0, err
	}

	return downloadrate, nil
}

func (dbc *DBConn) GetUploadRate(infoHash []byte) (float64, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    SELECT uploadrate_max
    FROM torrents
    WHERE info_hash = ?
    `
	var uploadrate float64

	err := dbc.db.QueryRow(query, infoHashStr).Scan(&uploadrate)
	if err != nil {
		return 0, err
	}

	return uploadrate, nil
}

func (dbc *DBConn) GetPath(infoHash []byte) (string, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    SELECT path
    FROM torrents
    WHERE info_hash = ?
    `
	var path string

	err := dbc.db.QueryRow(query, infoHashStr).Scan(&path)
	if err != nil {
		return "", err
	}

	return path, nil
}

func (dbc *DBConn) UpdatePath(infoHash []byte, path string) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    UPDATE torrents
    SET path = ?
    WHERE info_hash = ?
    `

	_, err := dbc.db.Exec(query, path, infoHashStr)
	if err != nil {
		return err
	}
	return nil
}

func (dbc *DBConn) GetStatus(infoHash []byte) (int, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    SELECT status
    FROM torrents
    WHERE info_hash = ?
    `
	var status int

	err := dbc.db.QueryRow(query, infoHashStr).Scan(&status)
	if err != nil {
		return 0, err
	}

	return status, nil
}

func (dbc *DBConn) UpdateUploadRate(infoHash []byte, rateKB float64) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    UPDATE torrents
    SET uploadrate_max = ?
    WHERE info_hash = ?
    `

	_, err := dbc.db.Exec(query, rateKB, infoHashStr)
	if err != nil {
		return err
	}

	dbc.logger.Debug(fmt.Sprintf("Updated upload rate for %x: %f", infoHash, rateKB))

	return nil
}

func (dbc *DBConn) UpdateDownloadRate(infoHash []byte, rateKB float64) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)

	const query = `
    UPDATE torrents
    SET downloadrate_max = ?
    WHERE info_hash = ?
    `

	_, err := dbc.db.Exec(query, rateKB, infoHashStr)
	if err != nil {
		return err
	}

	dbc.logger.Debug(fmt.Sprintf("Updated download rate for %x: %f", infoHash, rateKB))

	return nil
}

func (dbc *DBConn) GetBitfield(infoHash []byte) (*bitfield.Bitfield, error) {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()
	infoHashStr := string(infoHash)

	const query = `
	SELECT bitfield, bitfield_length
	FROM torrents
	WHERE info_hash = ?
	`

	var bitfieldLength int64
	var bitfieldBytes []byte
	err := dbc.db.QueryRow(query, infoHashStr).Scan(&bitfieldBytes, &bitfieldLength)
	if err != nil {
		return nil, err
	}
	bf := bitfield.FromBytes(bitfieldBytes, bitfieldLength)
	dbc.logger.Debug(fmt.Sprintf("Got bitfield for %x", infoHash))
	return bf, nil
}

func (dbc *DBConn) UpdateBitfield(infoHash []byte, bf *bitfield.Bitfield) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(infoHash)
	bitfieldBytes := bf.Bytes

	const query = `
	UPDATE torrents
	SET bitfield = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(query, bitfieldBytes, infoHashStr)
	if err != nil {
		return err
	}
	dbc.logger.Debug(fmt.Sprintf("Updated bitfield for %x", infoHash))
	return nil
}

func (dbc *DBConn) AddTorrent(tf *torrentfile.TorrentFile, bf *bitfield.Bitfield, path string, downloadrate_max int, uploadrate_max int, status int) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	infoHashStr := string(tf.InfoHash)
	torrentFileStr, err := tf.Bencode()
	if err != nil {
		return err
	}
	// path
	bitfieldBytes := bf.Bytes
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
    INSERT INTO torrents (info_hash, torrent_file, path, bitfield, bitfield_length, downloadrate_max, uploadrate_max, status)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `

	_, err = dbc.db.Exec(
		query,
		infoHashStr,
		torrentFileStr,
		path,
		bitfieldBytes,
		bitfieldLength,
		downloadrate_max,
		uploadrate_max,
		status,
	)
	if err != nil {
		return err
	}

	dbc.logger.Debug(fmt.Sprintf("Added torrent: %x", tf.InfoHash))

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

	dbc.logger.Debug(fmt.Sprintf("Deleted torrent: %x", infoHash))

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
	dbc.logger.Debug(fmt.Sprintf("Updated Torrent Status(%x): %d", infoHash, status))
	return nil
}
