package session

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/listenserver"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/trackerlist"
)

type Session struct {
	Path        string
	Bundle      *bundle.Bundle
	TrackerList *trackerlist.TrackerList
	tf          *torrentfile.TorrentFile

	maxDownloadRateKB       float64
	maxUploadRateKB         float64
	maxRequestsOutRate      float64
	maxResponsesOutRate     float64
	timeLastRequestsQueued  time.Time
	timeLastResponsesQueued time.Time

	Peers            []*peer.Peer
	UnconnectedPeers []*tracker.PeerInfo
	ConnectedPeers   []*tracker.PeerInfo
	ConnectingPeers  []*tracker.PeerInfo
	BannedPeers      []*tracker.PeerInfo

	listenPort uint16
	peerID     []byte
	maxPeers   int

	DownloadRateKB    float64
	UploadRateKB      float64
	TotalDownloadedKB float64
	TotalUploadedKB   float64

	BlockQueue    []*blockRequest
	BlockQueueMax int

	running bool
	Error   error
	mux     sync.Mutex

	logger *logger.Logger
}

type blockRequest struct {
	Info     bundle.BlockInfo
	Sent     bool
	Rejected [][]byte
	SentToID []byte
}

func (bra *blockRequest) Equal(brb *blockRequest) bool {
	return bra.Info.BeginOffset == brb.Info.BeginOffset &&
		bra.Info.PieceIndex == brb.Info.PieceIndex &&
		bra.Info.Length == brb.Info.Length
}

// Exported

func New(path string, tf *torrentfile.TorrentFile, bf *bitfield.Bitfield, listenPort uint16, peerID []byte) (*Session, error) {
	session := Session{
		Peers:                   make([]*peer.Peer, 0, 20),
		Path:                    path,
		tf:                      tf,
		listenPort:              listenPort,
		peerID:                  peerID,
		maxDownloadRateKB:       0,
		maxUploadRateKB:         0,
		timeLastRequestsQueued:  time.Now(),
		timeLastResponsesQueued: time.Now(),
		maxRequestsOutRate:      0,
		maxResponsesOutRate:     0,
		maxPeers:                10,
		DownloadRateKB:          0,
		UploadRateKB:            0,
		BlockQueue:              make([]*blockRequest, 0, 256),
		BlockQueueMax:           256,
		running:                 false,

		logger: logger.New(logger.ERROR, "Session"),
	}
	var bnd *bundle.Bundle
	bnd, err := bundle.Load(tf, bf, path)
	if os.IsNotExist(err) {
		bnd, err = bundle.Create(tf, path)
		if err != nil {
			session.Error = err
		}
	}
	session.Bundle = bnd
	session.TrackerList = trackerlist.New(
		tf.Announce,
		tf.AnnounceList,
		bnd.InfoHash,
		listenPort,
		(bnd.NumPieces-bnd.Bitfield.NumSet)*bnd.PieceLength,
		bnd.Complete,
		peerID,
	)
	return &session, nil
}

func (session *Session) Start() error {
	if session.Error != nil {
		return fmt.Errorf("cannot start session in error state: %v", session.Error)
	}
	session.running = true

	err := session.TrackerList.Start()
	if err != nil {
		return err
	}

	go session.run()

	return nil
}

func (session *Session) Stop() {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.running = false

	for _, req := range session.BlockQueue {
		session.Bundle.CancelBlock(&req.Info)
	}
	session.BlockQueue = make([]*blockRequest, 0, session.BlockQueueMax)
	for _, curPeer := range session.Peers {
		go curPeer.Close()
	}
	session.Peers = make([]*peer.Peer, 0, 20)
	session.ConnectedPeers = make([]*tracker.PeerInfo, 0, 20)
	session.ConnectingPeers = make([]*tracker.PeerInfo, 0, 20)
	session.UnconnectedPeers = make([]*tracker.PeerInfo, 0, 20)
	session.TrackerList.Stop()
}

func (session *Session) SetMaxDownloadRate(rateKB float64) {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.maxDownloadRateKB = rateKB
	session.maxRequestsOutRate = rateKB / float64(16)
}

func (session *Session) SetMaxUploadRate(rateKB float64) {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.maxUploadRateKB = rateKB
	session.maxResponsesOutRate = rateKB / float64(16)
}

func (session *Session) Recheck() error {
	if session.running {
		session.Stop()
	}
	err := session.Bundle.Recheck()
	if err != nil {
		return err
	}
	err = session.Start()
	return err
}

// Internal

func (session *Session) run() {
	for session.running {
		if session.Bundle.Complete { // Seeding
			// Assign responses
			session.checkRequests()
		} else { // Leeching

			session.sortPeers()
			session.calcRates()

			session.checkCancelled()

			// Assign blocks to queue from bundle
			session.getBlocksFromBundle()

			session.logger.Debug(fmt.Sprintf("Session blockqueue length: %d\n", len(session.BlockQueue)))

			session.assignParts()

			// Check for requests
			session.checkRequests()

			// Check for responses
			session.checkResponses()

			// Check for weird peers
			for _, curPeer := range session.Peers {
				if time.Now().After(curPeer.TimeConnected.Add(time.Second*20)) && curPeer.TotalBytesDownloaded < 1 {
					session.disconnectPeer(curPeer.RemotePeerID)
					session.BannedPeers = append(session.BannedPeers, &tracker.PeerInfo{IP: curPeer.RemoteIP, Port: curPeer.RemotePort})
				}
				if time.Now().After(curPeer.TimeLastReceived.Add(time.Second*5)) && curPeer.RemoteChoking {
					session.disconnectPeer(curPeer.RemotePeerID)
					session.BannedPeers = append(session.BannedPeers, &tracker.PeerInfo{IP: curPeer.RemoteIP, Port: curPeer.RemotePort})
				}
			}

			for _, curPeer := range session.Peers {
				if float64(session.Bundle.Bitfield.NumDiff(curPeer.RemoteBitfield))/float64(session.Bundle.Bitfield.Len()) > 20 {
					session.disconnectPeer(curPeer.RemotePeerID)
					session.BannedPeers = append(session.BannedPeers, &tracker.PeerInfo{IP: curPeer.RemoteIP, Port: curPeer.RemotePort})
				}
			}

			// Check for new peers connecting

			// Check for new peers to connect to
			session.UpdatePeerInfo()
			session.logger.Debug(fmt.Sprintf("Current peers (max %d): \nConnected: %d\nConnecting: %d\nUnconnected: %d\n", session.maxPeers, len(session.ConnectedPeers), len(session.ConnectingPeers), len(session.UnconnectedPeers)))
			if len(session.ConnectedPeers)+len(session.ConnectingPeers) < session.maxPeers && len(session.UnconnectedPeers) > 0 {
				go session.findNewPeer()
			}

			session.logger.Debug(fmt.Sprintf("Rates: Down: %f KB/s Up: %f KB/s\n", session.DownloadRateKB, session.UploadRateKB))

			time.Sleep(50 * time.Millisecond)
		}
	}
}

func sanitize(s string) string {
	ret := ""
	for _, r := range s {
		if unicode.IsPrint(r) {
			ret = ret + string(r)
		}
	}
	return ret
}

func (session *Session) UpdatePeerInfo() {
	session.mux.Lock()
	defer session.mux.Unlock()
	newPeers := session.TrackerList.GetPeers()

	for _, newPeer := range newPeers {
		found := false
		for _, connectedPeer := range session.ConnectedPeers {
			if connectedPeer.IP == newPeer.IP && connectedPeer.Port == newPeer.Port {
				found = true
				break
			}
		}
		if found {
			continue
		}
		for _, unconnectedPeer := range session.UnconnectedPeers {
			if unconnectedPeer.IP == newPeer.IP && unconnectedPeer.Port == newPeer.Port {
				found = true
				break
			}
		}
		if found {
			continue
		}
		for _, connectingPeer := range session.ConnectingPeers {
			if connectingPeer.IP == newPeer.IP && connectingPeer.Port == newPeer.Port {
				found = true
				break
			}
		}
		if found {
			continue
		}
		session.UnconnectedPeers = append(session.UnconnectedPeers, newPeer)
	}
}

func (session *Session) getBlocksFromBundle() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if len(session.BlockQueue) < session.BlockQueueMax {
		space := session.BlockQueueMax - len(session.BlockQueue)
		blocksToRequest := session.Bundle.NextNBlocks(space)
		requestsToAdd := make([]*blockRequest, 0, 150)
		for _, block := range blocksToRequest {
			newReq := &blockRequest{
				Info:     *block,
				Rejected: make([][]byte, 0, 20),
				Sent:     false,
			}
			requestsToAdd = append(requestsToAdd, newReq)
		}
		session.BlockQueue = append(session.BlockQueue, requestsToAdd...)
	}
}

func (session *Session) checkRequests() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if session.maxUploadRateKB == 0 || session.UploadRateKB < session.maxUploadRateKB {
		now := time.Now()
		added := false
		timeSinceLastResponsesQueues := now.Sub(session.timeLastResponsesQueued)
		numBlocksCanQueue := int(float64(timeSinceLastResponsesQueues.Milliseconds()/1000) * session.maxResponsesOutRate)
		for _, curPeer := range session.Peers {
			numRequestsIn := curPeer.NumRequestsIn()
			if session.maxUploadRateKB != 0 {
				if numBlocksCanQueue < numRequestsIn {
					numRequestsIn = numBlocksCanQueue
				}
			}
			requests := curPeer.GetRequestsIn(numRequestsIn)
			for _, req := range requests {
				if curPeer.NumResponsesOut() == curPeer.ResponsesOutMax {
					break
				}
				nextResponse, err := session.Bundle.GetBlock(&req.Info)
				if err != nil {
					session.logger.Error(fmt.Sprintf("err loading block: (%d, %d, %d) for response: %v\n", req.Info.PieceIndex, req.Info.BeginOffset, req.Info.Length, err))
					continue
				}
				curPeer.QueueResponseOut(
					uint32(req.Info.PieceIndex),
					uint32(req.Info.BeginOffset),
					nextResponse,
				)
				added = true
			}
		}
		if added {
			session.timeLastResponsesQueued = now
		}
	}
}

func (session *Session) checkResponses() {
	session.mux.Lock()
	defer session.mux.Unlock()

	for _, curPeer := range session.Peers {
		if curPeer.NumResponsesIn() > 0 {
			responses := curPeer.GetResponsesIn()
			for len(responses) > 0 {
				err := session.Bundle.WriteBlock(
					int64(responses[0].PieceIndex),
					int64(responses[0].BeginOffset),
					responses[0].Block,
				)
				bi := bundle.BlockInfo{PieceIndex: int64(responses[0].PieceIndex), BeginOffset: int64(responses[0].BeginOffset), Length: int64(len(responses[0].Block))}
				if err != nil {
					session.logger.Error(fmt.Sprintf("error writing response (%d, %d) from peer(%s): %v\n", responses[0].PieceIndex, responses[0].BeginOffset, curPeer.RemotePeerID, err))
					session.Bundle.CancelBlock(&bi)
				}
				i := 0
				req := &blockRequest{Info: bi, Sent: false}
				for i < len(session.BlockQueue) {
					if session.BlockQueue[i].Equal(req) {
						session.BlockQueue = append(session.BlockQueue[:i], session.BlockQueue[i+1:]...)
						break
					}
					i++
				}
				responses = responses[1:]
			}
		}
	}
}

func (session *Session) checkCancelled() {
	session.mux.Lock()
	defer session.mux.Unlock()

	// Check for cancelled pieces
	for _, curPeer := range session.Peers {
		if curPeer.NumRequestsCancelled() > 0 {
			cancelled := curPeer.GetCancelledRequests()
			if cancelled == nil {
				continue
			}
			for _, req := range cancelled {
				i := 0
				for i < len(session.BlockQueue) {
					if req.Info.PieceIndex == session.BlockQueue[i].Info.PieceIndex &&
						req.Info.BeginOffset == session.BlockQueue[i].Info.BeginOffset &&
						req.Info.Length == session.BlockQueue[i].Info.Length {
						session.BlockQueue[i].Rejected = append(session.BlockQueue[i].Rejected, curPeer.RemotePeerID)
						session.BlockQueue[i].Sent = false
						session.BlockQueue[i].SentToID = make([]byte, 0)
						break
					}
					i++
				}
			}
		}
	}
}

func (session *Session) assignParts() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if session.maxDownloadRateKB == 0 || session.DownloadRateKB < session.maxDownloadRateKB {
		now := time.Now()
		timeSinceLastRequestsQueued := now.Sub(session.timeLastRequestsQueued)
		numBlocksCanQueue := int(float64(timeSinceLastRequestsQueued.Milliseconds()/1000) * session.maxRequestsOutRate)
		added := false
		// Assign requests to peers
		for peerIndex, curPeer := range session.Peers {
			numCanAdd := curPeer.NumRequestsCanAdd()
			if curPeer.RemoteChoking || numCanAdd == 0 {
				continue
			}
			if session.maxDownloadRateKB != 0 {
				if numCanAdd > int(numBlocksCanQueue) {
					numCanAdd = int(numBlocksCanQueue)
				}
			}
			if curPeer.NumRequestsCancelled() < curPeer.RequestsOutMax/4 &&
				numCanAdd > curPeer.RequestsOutMax/2 &&
				curPeer.RequestsOutMax < 256 {
				curPeer.SetMaxRequests(curPeer.RequestsOutMax * 2)
			}
			if numCanAdd > 0 {
				numAdded := 0
				curRequestIndex := 0
				for numAdded < numCanAdd && curRequestIndex < len(session.BlockQueue) {
					curBlockReq := session.BlockQueue[curRequestIndex]
					if curBlockReq.Sent {
						curRequestIndex++
						continue
					}
					rejected := false
					for _, rej := range curBlockReq.Rejected {
						if bytes.Equal(rej, curPeer.RemotePeerID) {
							rejected = true
							break
						}
					}
					if !rejected && curPeer.HasPiece(curBlockReq.Info.PieceIndex) {
						curPeer.QueueRequestOut(curBlockReq.Info)
						session.BlockQueue[curRequestIndex].Sent = true
						session.BlockQueue[curRequestIndex].SentToID = curPeer.RemotePeerID
						added = true
						numAdded++
					}
					if rejected && peerIndex == len(session.Peers) {
						session.BlockQueue[curRequestIndex].Rejected = make([][]byte, 0, 20)
					}
					curRequestIndex++
				}
			}
		}
		if added {
			session.timeLastRequestsQueued = now
		}
	}
}

func (session *Session) fetchBlockInfos() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if !session.Bundle.Complete {
		blocksNeeded := session.BlockQueueMax - len(session.BlockQueue)
		addedBlocks := session.Bundle.NextNBlocks(blocksNeeded)
		for _, bi := range addedBlocks {
			session.BlockQueue = append(session.BlockQueue, &blockRequest{Info: *bi, Sent: false})
		}
	}
}

func (session *Session) findNewPeer() {
	if len(session.UnconnectedPeers) == 0 {
		return
	}
	j := 0
	for j < len(session.UnconnectedPeers) {
		session.mux.Lock()
		peerInfo := session.UnconnectedPeers[j]
		session.ConnectingPeers = append(session.ConnectingPeers, peerInfo)
		session.UnconnectedPeers = append(session.UnconnectedPeers[:j], session.UnconnectedPeers[j+1:]...)
		session.ConnectingPeers = append(session.ConnectingPeers, peerInfo)
		session.mux.Unlock()
		session.logger.Debug(fmt.Sprintf("session trying to connect to peer(%s:%d)...\n", peerInfo.IP, peerInfo.Port))
		err := session.connectPeer(peerInfo)
		if err != nil {
			session.mux.Lock()
			i := 0
			for i < len(session.ConnectingPeers) {
				if session.ConnectingPeers[i].IP == peerInfo.IP && session.ConnectingPeers[i].Port == peerInfo.Port {
					session.ConnectingPeers = append(session.ConnectingPeers[:i], session.ConnectingPeers[i+1:]...)
					session.UnconnectedPeers = append(session.UnconnectedPeers, peerInfo)
					break
				}
				i++
			}
			j++
			session.mux.Unlock()
			session.logger.Debug(fmt.Sprintf("error connecting to peer: %v\n", err))
		} else {
			session.mux.Lock()
			i := 0
			for i < len(session.ConnectingPeers) {
				if session.ConnectingPeers[i].IP == peerInfo.IP && session.ConnectingPeers[i].Port == peerInfo.Port {
					session.ConnectingPeers = append(session.ConnectingPeers[:i], session.ConnectingPeers[i+1:]...)
					session.ConnectedPeers = append(session.ConnectedPeers, peerInfo)
					break
				}
				i++
			}
			session.mux.Unlock()
			break
		}
	}
}

func (session *Session) AddPeer(incomingPeer *listenserver.IncomingPeer) {
	if len(session.ConnectedPeers)+len(session.ConnectingPeers) >= session.maxPeers {
		session.logger.Error(fmt.Sprintf("Error adding peer(%s): session full", incomingPeer.Conn.RemoteAddr().String()))
		return
	}
	peer, err := peer.Add(incomingPeer.Conn, incomingPeer.InfoHash, session.peerID, session.Bundle.Bitfield, session.Bundle.NumPieces)
	if err != nil {
		session.logger.Error(fmt.Sprintf("Error adding peer(%s): %v", incomingPeer.Conn.RemoteAddr().String(), err))
	}
	peerAddrParts := strings.Split(incomingPeer.Conn.RemoteAddr().String(), ":")
	peerPort, err := strconv.Atoi(peerAddrParts[1])
	if err != nil {
		session.logger.Error(fmt.Sprintf("Error adding peer(%s): %v", incomingPeer.Conn.RemoteAddr().String(), err))
	}
	session.mux.Lock()
	session.Peers = append(session.Peers, peer)
	session.ConnectedPeers = append(session.ConnectedPeers, &tracker.PeerInfo{IP: peerAddrParts[0], Port: peerPort})
	session.mux.Unlock()
}

func (session *Session) connectPeer(pi *tracker.PeerInfo) error {
	for _, peer := range session.Peers {
		if peer.RemoteIP == pi.IP && peer.RemotePort == pi.Port {
			return errors.New("peer already connected")
		}
	}
	for _, peerinfo := range session.BannedPeers {
		if peerinfo.IP == pi.IP && peerinfo.Port == pi.Port {
			return errors.New("peer banned")
		}
	}
	peer, err := peer.Connect(
		pi.IP,
		pi.Port,
		session.Bundle.InfoHash,
		session.peerID,
		session.Bundle.Bitfield,
		session.Bundle.NumPieces,
	)
	if err != nil {
		return err
	}
	session.Peers = append(session.Peers, peer)
	return nil
}

func (session *Session) disconnectPeer(peerID []byte) error {
	session.mux.Lock()
	defer session.mux.Unlock()

	var peer *peer.Peer
	var peerIndex int
	for i, curPeer := range session.Peers {
		if bytes.Equal(curPeer.RemotePeerID, peerID) {
			peer = curPeer
			peerIndex = i
		}
	}
	if peer == nil {
		return errors.New("ID not in peers list")
	}
	peer.Close()
	for i := range session.BlockQueue {
		if session.BlockQueue[i].Sent && bytes.Equal(session.BlockQueue[i].SentToID, peerID) {
			session.BlockQueue[i].Sent = false
			session.BlockQueue[i].SentToID = make([]byte, 0)
		}
	}
	session.Peers = append(session.Peers[:peerIndex], session.Peers[peerIndex+1:]...)
	return nil
}

func (session *Session) sortPeers() {
	session.mux.Lock()
	defer session.mux.Unlock()

	sort.Slice(session.Peers, func(i, j int) bool {
		return session.Peers[i].TotalBytesDownloaded > session.Peers[j].TotalBytesDownloaded
	})
}

func (session *Session) calcRates() {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.DownloadRateKB = 0
	session.UploadRateKB = 0
	session.TotalDownloadedKB = 0
	session.TotalUploadedKB = 0
	for _, peer := range session.Peers {
		session.TotalDownloadedKB += float64(peer.TotalBytesDownloaded) / 1024
		session.TotalUploadedKB += float64(peer.TotalBytesUploaded) / 1024
		session.DownloadRateKB += peer.DownloadRateKB
		session.UploadRateKB += peer.UploadRateKB
	}
}
