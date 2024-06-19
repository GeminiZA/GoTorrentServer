package database

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type Record struct {
	infoHashHex string
	bencodedFile string
	path string
	bitFieldHex string
	bitfieldLength int
}

func (dbc *DBConn) AddTorrent(tf *torrentfile.TorrentFile, bundlePath string, bitfield []byte, bitfieldLength int) error {
	dbc.mux.Lock()
	defer dbc.mux.Unlock()

	bencodedTF, err := tf.Bencode()
	if err != nil {
		return err
	}

	infoHashHex := fmt.Sprintf("%x", tf.InfoHash)

	bitfieldHex := fmt.Sprintf("%x", bitfield)

	const addTorrentQuery = `
	INSERT INTO torrents (info_hash_hex, torrent_file, path, bitfield_hex, bitfield_length)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (info_hash) DO UPDATE SET
	torrent_file = EXCLUUDED.torrent_file,
	path = EXCLUDED.path,
	bitfield = EXCLUDED.bitfield,
	bitfield_length = EXCLUDED.bitfield_length
	`

	_, err = dbc.db.Exec(addTorrentQuery, infoHashHex, bencodedTF, bundlePath, bitfieldHex, bitfieldLength)
	if err != nil {
		return nil
	}

	return nil
}