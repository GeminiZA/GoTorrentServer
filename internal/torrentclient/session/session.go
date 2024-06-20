package session

import (
	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type Session struct {
	bundle 			*bundle.Bundle
	dbc 			*database.DBConn
	tracker 		*tracker.Tracker
	tf 				*torrentfile.TorrentFile

	maxDownRateKB 	int
	maxUpRateKB 	int

	peers			[]*peer.Peer
}

// Exported

func New() (*Session, error) {
	return nil, nil
}

func (session *Session) Start() error {
	return nil
}

func (session *Session) Pause() error {
	return nil
}

func (session *Session) SetMaxDown(rateKB int) error {
	return nil
}

func (session *Session) SetMaxUpRate(rateKB int) error {
	return nil
}

// Internal

func (session *Session) run() {

}