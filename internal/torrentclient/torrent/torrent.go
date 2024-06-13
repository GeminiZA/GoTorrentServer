package torrent

import (
	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/outsocket"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type Torrent struct {
	Peers []*peer.Peer
	outSocket *outsocket.OutSocket
	tracker *tracker.Tracker
	db *database.DBConn
}

func Start() {

}