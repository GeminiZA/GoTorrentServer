// TODO:
// Implemenent connections listenserver

package client

import (
	"bytes"
	"errors"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/listenserver"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

const (
	ListenPort uint16 = 6881
	PeerID     string = "-TR2940-6oFw2M6BdUkY"
)

type TorrentClient struct {
	sessions     []*session.Session
	bitfields    []*bitfield.Bitfield
	dbc          *database.DBConn
	listenServer *listenserver.ListenServer
	// peerInChan   chan<- *peer.Peer
	running bool
	mux     sync.Mutex
}

func Start() (*TorrentClient, error) {
	var err error
	client := &TorrentClient{
		sessions: make([]*session.Session, 0),
		// peerInChan: make(chan<- *peer.Peer, 10),
	}
	client.dbc, err = database.Connect()
	if err != nil {
		return nil, err
	}
	client.running = true
	// client.listenServer, err = listenserver.New(ListenPort, client.peerInChan)
	//	if err != nil {
	// 	return client, err
	// }
	return client, nil
}

func (client *TorrentClient) Stop() error {
	client.mux.Lock()
	defer client.mux.Unlock()

	client.running = false
	for _, sesh := range client.sessions {
		sesh.Stop()
	}
	client.dbc.Disconnect()
	return nil
}

func (client *TorrentClient) AddTorrentFromFile(torrentfilePath string, targetPath string, start bool) error {
	client.mux.Lock()
	defer client.mux.Unlock()
	tf := torrentfile.New()

	err := tf.ParseFile(torrentfilePath)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, tf.InfoHash) {
			return errors.New("torrent already exists in client")
		}
	}
	newSesh, err := session.New(targetPath, tf, ListenPort, PeerID)
	if err != nil {
		return err
	}
	client.sessions = append(client.sessions, newSesh)
	client.bitfields = append(client.bitfields, newSesh.Bundle.Bitfield.Clone())
	if start {
		err = newSesh.Start()
		if err != nil {
			return err
		}
	}
	return nil
}

func AddTorrentFromURL(magnet string) error {
	// Not implemented
	return errors.New("not yet implemented")
}

func (client *TorrentClient) runClient() {
	for client.running {
		client.updateDatabase()
		client.processIncomingClients()
		time.Sleep(time.Millisecond * 1000)
	}
}

func (client *TorrentClient) updateDatabase() {
	client.mux.Lock()
	defer client.mux.Unlock()

	for i := range client.sessions {
		if !client.sessions[i].Bundle.Bitfield.Equals(client.bitfields[i]) {
			client.dbc.UpdateBitfield(client.sessions[i].Bundle.InfoHash, client.sessions[i].Bundle.Bitfield)
		}
	}
}

func (client *TorrentClient) processIncomingClients() {
	// Not implemented
}
