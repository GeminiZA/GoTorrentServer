package client

import "github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"

type TorrentClient struct {
	sessions []*session.Session
}
