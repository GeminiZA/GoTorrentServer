package client

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/listenport"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type TorrentClient struct {
	DB *database.DBConn
	Trackers []*tracker.Tracker
	TorrentFiles []*torrentfile.TorrentFile
	started bool
	DBConnected bool
	lp *listenport.ListenPort
	newPeerChan chan<-*peer.Peer
	lpPort int
	running bool
	curNumPeers int
	maxPeers int
}

func New(listenPort int) (*TorrentClient, error) {
	var client TorrentClient
	var err error
	client.started = false
	client.DBConnected = false
	client.lp, err = listenport.New(listenPort, client.newPeerChan)
	if err != nil {
		return nil, err
	}
	return &client, nil
}

func (tc *TorrentClient) Start() error {
	var err error
	tc.DB, err = database.Connect()
	if err != nil {
		return err
	}
	tc.running = true
	return nil
}

func (tc *TorrentClient) Listen() error {
	var errCh chan<-error
	go tc.lp.Start(errCh)
	defer tc.lp.Stop()
	for tc.running {
		newPeer := <-tc.newPeerChan
	}
}

func (tc *TorrentClient) Stop() []error {
	var errs []error
	for _, tr := range tc.Trackers {
		err := tr.Stop()
		if err != nil {
			errs = append(errs, fmt.Errorf("tracker (%s) error: %s", tr.InfoHash, err))
		}
	}
	if err := tc.DB.Disconnect(); err != nil {
		errs = append(errs, err)
	}
	return errs
}

func (tc *TorrentClient) AddFile(path string) error {
	var err error
	//Parse torrent file
	tf, err := torrentfile.ParseFile(path)
	if err != nil {
		return err
	}

	tc.TorrentFiles = append(tc.TorrentFiles, tf)

	//Add to db
	err = tc.DB.AddTorrentFile(string(tf.InfoHash), tf)
	if err != nil {
		return err
	}

	port := 6681
	peerID :=  "-AZ2060-6wfG2wk6wWLc"


	tr := tracker.New(tf.Announce, tf.InfoHash, port, 0, 0, tf.Info.Length, peerID)
	tc.Trackers = append(tc.Trackers, tr)
	err = tr.Start()
	if err != nil {
		return err
	}

	return nil
}