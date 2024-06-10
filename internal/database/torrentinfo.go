package database

import (
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

func (dbc *DBConn) AddTorrentFile(infoHash string, tf *torrentfile.TorrentFile) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	bencodedFile, err := tf.Bencode()
	if err != nil {
		return err
	}

	const insert string = `
	INSERT INTO torrentinfo (info_hash, torrent_file)
	VALUES (?, ?)
	`

	_, err = dbc.db.Exec(insert, infoHash, bencodedFile)

	return err
}

func (dbc *DBConn) getTorrentFile(infoHash string) (*torrentfile.TorrentFile, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT torrent_file
	FROM torrents
	WHERE info_hash = ?
	`
	var bencodedFile []byte
	err := dbc.db.QueryRow(query, infoHash).Scan(bencodedFile)
	if err != nil {
		return nil, err
	}
	tf, err := torrentfile.ParseFileString(&bencodedFile)
	return tf, err
}