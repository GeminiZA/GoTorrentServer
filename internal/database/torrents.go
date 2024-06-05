package database

import (
	"database/sql"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DBConn struct {
	db *sql.DB
	mu sync.Mutex
}

type TargetFile struct {
	Path string
	InfoHash string
	Time string
	Description string
	TrackerURL string
}

func Connect() (*DBConn, error) {
	const file string = "gotorrentserver.db"
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}
	const create string = `
	CREATE TABLE IF NOT EXISTS torrents (
		info_hash STRING NOT NULL PRIMARY KEY,
		time DATETIME NOT NULL,
		tracker_url STRING NOT NULL,
		path STRING NOT NULL,
		description TEXT
	);`
	if _, err := db.Exec(create); err != nil {
		return nil, err
	}
	return &DBConn{
		db: db,
	}, nil
}

func (dbc *DBConn) AddFile(tf TargetFile) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const insert string = `
	INSERT INTO torrents (info_hash, time, tracker_url, path, description)
	VALUES (?, ?, ?, ?, ?)
	`

	_, err := dbc.db.Exec(insert, tf.InfoHash, time.Now(), tf.TrackerURL, tf.Path, tf.Description)

	return err
}

func (dbc *DBConn) AddFileStrings(infoHash string, trackerUrl string, path string, description string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	tf := TargetFile{
		InfoHash: infoHash,
		Time: time.Now().String(),
		TrackerURL: trackerUrl,
		Path: path,
		Description: description,
	}

	const insert string = `
	INSERT INTO torrents (info_hash, time, tracker_url, path, description)
	VALUES (?, ?, ?, ?, ?)
	`

	_, err := dbc.db.Exec(insert, tf.InfoHash, tf.Time, tf.TrackerURL, tf.Path)

	return err
}

func (dbc *DBConn) GetFile(infoHash string) (*TargetFile, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT info_hash, time, tracker_url, path, description
	FROM torrents
	WHERE info_hash = ?
	`
	var tf TargetFile
	err := dbc.db.QueryRow(query, infoHash).Scan(&tf.InfoHash, &tf.Time, &tf.TrackerURL, &tf.Path, &tf.Description)

	if err != nil {
		return nil, err
	}

	return &tf, nil
}

func (dbc *DBConn) GetPath(infoHash string) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT path
	FROM torrents
	WHERE info_hash = ?
	`

	var path string
	err := dbc.db.QueryRow(query, infoHash).Scan(&path)

	if err != nil {
		return "", err
	}

	return path, nil
}

func (dbc *DBConn) GetTrackerURL(infoHash string) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT tracker_url
	FROM torrents
	WHERE info_hash = ?
	`

	var trackerUrl string
	err := dbc.db.QueryRow(query, infoHash).Scan(&trackerUrl)

	if err != nil {
		return "", err
	}

	return trackerUrl, nil
}

func (dbc *DBConn) GetAllFiles() ([]*TargetFile, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT info_hash, time, tracker_url, path, description
	FROM torrents
	`
	rows, err := dbc.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var files []*TargetFile
	for rows.Next() {
		var tf TargetFile
		if err := rows.Scan(&tf.InfoHash, &tf.Time, &tf.TrackerURL, &tf.Path, &tf.Description); err != nil {
			return nil, err
		}
		files = append(files, &tf)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}

func (dbc *DBConn) DeleteFile(infoHash string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const delete string = `
	DELETE FROM torrents
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(delete, infoHash)

	return err
}

func (dbc *DBConn) Disconnect() error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	return dbc.db.Close()
}