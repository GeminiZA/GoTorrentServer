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

	peerIDList []string
	peersMap   map[string]*peer.Peer

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
		peerIDList:     make([]string, 0),
		peersMap:       make(map[string]*peer.Peer),
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
		if len(session.peerIDList) >= session.maxPeers {
			break
		}
		if _, exists := session.peersMap[peerInfo.PeerID]; !exists {
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
	for len(session.peerIDList) > 0 {
		session.disconnectPeer(session.peerIDList[0])
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
	blocksNeeded := make([]blockRequest, 0)
	for _, bi := range session.blockQueue {
		if !bi.sent {
			blocksNeeded = append(blocksNeeded, bi)
		}
	}
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
	_, exists := session.peersMap[pi.PeerID]
	if exists {
		return errors.New("peer already connected")
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
	session.peerIDList = append(session.peerIDList, pi.PeerID)
	session.peersMap[pi.PeerID] = peer
	return nil
}

func (session *Session) disconnectPeer(peerID string) error {
	curPeer, exists := session.peersMap[peerID]
	if !exists {
		return errors.New("peer not connected")
	}
	curPeer.Close()
	i := 0
	for session.peerIDList[i] != peerID {
		i++
	}
	if i >= len(session.peerIDList) {
		return errors.New("peer id not found in list")
	}
	if i == 0 {
		session.peerID = session.peerID[1:]
	} else if i == len(session.peerIDList)-1 {
		session.peerIDList = session.peerIDList[:len(session.peerIDList)-2]
	} else {
		session.peerIDList = append(session.peerIDList[0:i], session.peerIDList[i+1:]...)
	}
	return nil
}

func (session *Session) sortPeers() {
	sort.Slice(session.peerIDList, func(i, j int) bool {
		peerI, existsI := session.peersMap[session.peerIDList[i]]
		peerJ, existsJ := session.peersMap[session.peerIDList[j]]
		if !existsI || !existsJ {
			return false
		}
		return peerI.DownloadRateKB < peerJ.DownloadRateKB
	})
}

func (session *Session) calcRates() {
	session.downloadRateKB = 0
	session.uploadRateKB = 0
	for _, peer := range session.peersMap {
		session.downloadRateKB += peer.DownloadRateKB
		session.uploadRateKB += peer.UploadRateKB
	}
}

