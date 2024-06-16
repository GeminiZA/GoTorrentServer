package session

import (
	"errors"
	"fmt"
	"net"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type Session struct {
	tf *torrentfile.TorrentFile
	peers []*peer.Peer
	bundle *bundle.Bundle
	tracker *tracker.Tracker
	maxConnections int
	running bool
}

func New(torrentFile *torrentfile.TorrentFile, bundle *bundle.Bundle, maxCon int) (*Session, error) {
	session := Session{tf: torrentFile, peers: make([]*peer.Peer, 0), bundle: bundle, maxConnections: maxCon}
	return &session, nil
}

func (session *Session) Start(outport int, myPeerId string) error {
	if session.tf == nil || session.peers == nil || session.bundle == nil {
		return errors.New("session not instantiated properly")
	}
	fmt.Printf("Connection to tracker: %s\ninfohash: %s, outport: %d, left: %d, myPeerID: %s\n", session.tf.Announce, string(session.tf.InfoHash), outport, session.bundle.BytesLeft(), myPeerId)
	session.tracker = tracker.New(session.tf.Announce, session.tf.InfoHash, outport, 0, 0, session.bundle.BytesLeft(), myPeerId)
	err := session.tracker.Start()
	if err != nil {
		return err
	}
	for _, peerDict := range session.tracker.Peers {
		peerIP, ok := peerDict["ip"].(string)
		if !ok {
			return errors.New("peer IP not string")
		}
		peerPort, ok := peerDict["port"].(int64)
		if !ok {
			return errors.New("peer port not int64")
		}
		peerID, ok := peerDict["peer id"].(string)
		if !ok {
			return errors.New("peer ID not string")
		}
		peer, err := peer.Connect(session.tf.InfoHash, session.bundle.NumPieces, peerIP, int(peerPort), peerID, myPeerId)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				fmt.Printf("Peer (%s) timed out on connect...\n", peerID)
				continue
			} else {
				return err
			}
		}
		session.peers = append(session.peers, peer)
		fmt.Printf("Session added peer: %s\n", peer.PeerID)
	}
	fmt.Println("Peers List:")
	for _, peer := range session.peers {
		fmt.Printf("ID: %s,\tIP: %s,\tPieces:%d\n", peer.PeerID, peer.IP, peer.NumPieces())
	}
	session.running = true
	//go session.handleSession()
	return nil
}

func (session *Session) TestBlock() {
	if session.running {
		bi, err := session.bundle.NextBlock()
		if err != nil {
			panic(err)
		}
		for _, peer := range session.peers {
			if peer.IsConnected() && peer.HasPiece(bi.PieceIndex) {
				fmt.Printf("Added block req to %s\n", peer.PeerID)
				peer.DownloadBlock(bi)
				break
			}
		}
	}
}

func (session *Session) handleSession() {
	for session.running {
		bi, err := session.bundle.NextBlock()
		if err != nil {
			panic(err)
		}
		for _, peer := range session.peers {
			if peer.IsConnected() && peer.HasPiece(bi.PieceIndex) {
				fmt.Printf("Added block req to %s\n", peer.PeerID)
				peer.DownloadBlock(bi)
				break
			}
		}
	}
}

func (session *Session) Close() {
	session.running = false
	for _, peer := range session.peers {
		peer.Close()
	}
	session.tracker.Stop()
}