package session

import (
	"errors"
	"fmt"
	"sort"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type blockRequest struct {
	info *bundle.BlockInfo
	sent bool
}

type Session struct {
	bundle  *bundle.Bundle
	dbc     *database.DBConn
	tracker *tracker.Tracker
	tf      *torrentfile.TorrentFile

	maxDownRateKB float64
	maxUpRateKB   float64

	peers []*peer.Peer

	listenPort int
	peerID     string
	maxPeers   int

	downloadRateKB float64
	uploadRateKB   float64

	blockQueue    []blockRequest
	blockQueueMax int
}

// Exported

func New(bnd *bundle.Bundle, dbc *database.DBConn, tf *torrentfile.TorrentFile, listenPort int, peerID string) (*Session, error) {
	session := Session{
		peers:          make([]*peer.Peer, 0),
		bundle:         bnd,
		tf:             tf,
		dbc:            dbc,
		listenPort:     listenPort,
		peerID:         peerID,
		maxDownRateKB:  0,
		maxUpRateKB:    0,
		maxPeers:       1,
		downloadRateKB: 0,
		uploadRateKB:   0,
		blockQueue:     make([]blockRequest, 0),
		blockQueueMax:  50,
	}
	session.tracker = tracker.New(tf.Announce, tf.InfoHash, listenPort, 0, 0, 0, peerID)
	return &session, nil
}

func (session *Session) Start() error {
	err := session.tracker.Start()
	if err != nil {
		return err
	}
	for _, peerInfo := range session.tracker.Peers {
		if len(session.peers) >= session.maxPeers {
			break
		}
		exists := false
		for _, peer := range session.peers {
			if peer.RemotePeerID == peerInfo.PeerID {
				exists = true
				break
			}
		}
		if !exists {
			session.connectPeer(peerInfo)
		}
	}

	go session.run()

	return nil
}

func (session *Session) Stop() {
	err := session.tracker.Stop()
	if err != nil {
		fmt.Printf("Error stopping tracker: %v\n", err)
	}
	for len(session.peers) > 0 {
		session.peers[0].Close()
		session.peers = session.peers[1:]
	}
}

func (session *Session) SetMaxDown(rateKB float64) {
	session.maxDownRateKB = rateKB
}

func (session *Session) SetMaxUpRate(rateKB float64) {
	session.maxUpRateKB = rateKB
}

// Internal

func (session *Session) run() {
	if !session.bundle.Complete {
		if len(session.blockQueue) < session.blockQueueMax {
			session.fetchBlockInfos()
		}
		session.assignParts()
	}
}

func (session *Session) assignParts() {
	session.sortPeers()
}

func (session *Session) fetchBlockInfos() {
	if !session.bundle.Complete {
		blocksNeeded := session.blockQueueMax - len(session.blockQueue)
		addedBlocks := session.bundle.NextNBlocks(blocksNeeded)
		for _, bi := range addedBlocks {
			session.blockQueue = append(session.blockQueue, blockRequest{info: bi, sent: false})
		}
	}
}

func (session *Session) connectPeer(pi tracker.PeerInfo) error {
	for _, peer := range session.peers {
		if peer.RemotePeerID == pi.PeerID {
			return errors.New("peer already connected")
		}
	}
	peer, err := peer.Connect(
		pi.PeerID,
		pi.IP,
		pi.Port,
		session.bundle.InfoHash,
		session.peerID,
		session.bundle.Bitfield,
		session.bundle.NumPieces,
	)
	if err != nil {
		err = fmt.Errorf("error connecting to peer(%s): %s", pi.PeerID, err)
		return err
	}
	session.peers = append(session.peers, peer)
	return nil
}

func (session *Session) disconnectPeer(peerID string) error {
	var peer *peer.Peer
	var peerIndex int
	for i, curPeer := range session.peers {
		if curPeer.RemotePeerID == peerID {
			peer = curPeer
			peerIndex = i
		}
	}
	if peer == nil {
		return errors.New("ID not in peers list")
	}
	peer.Close()
	session.peers = append(session.peers[:peerIndex], session.peers[peerIndex+1:]...)
	return nil
}

func (session *Session) sortPeers() {
	sort.Slice(session.peers, func(i, j int) bool {
		return session.peers[i].DownloadRateKB < session.peers[j].DownloadRateKB
	})
}

func (session *Session) calcRates() {
	session.downloadRateKB = 0
	session.uploadRateKB = 0
	for _, peer := range session.peers {
		session.downloadRateKB += peer.DownloadRateKB
		session.uploadRateKB += peer.UploadRateKB
	}
}
