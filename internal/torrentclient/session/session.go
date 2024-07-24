package session

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/trackerlist"
)

type Session struct {
	bundle      *bundle.Bundle
	dbc         *database.DBConn
	trackerList *trackerlist.TrackerList
	tf          *torrentfile.TorrentFile

	maxDownloadRateKB       float64
	maxUploadRateKB         float64
	maxRequestsOutRate      float64
	maxResponsesOutRate     float64
	timeLastRequestsQueued  time.Time
	timeLastResponsesQueued time.Time

	Peers           []*peer.Peer
	PeersInfo       []*tracker.PeerInfo
	ConnectingPeers []*tracker.PeerInfo

	listenPort uint16
	peerID     string
	maxPeers   int
	numPeers   int

	downloadRateKB float64
	uploadRateKB   float64

	blockQueue    []*peer.BlockRequest
	blockQueueMax int
	numBlocksSent int

	running bool
	mux     sync.Mutex
}

// Exported

func New(bnd *bundle.Bundle, dbc *database.DBConn, tf *torrentfile.TorrentFile, listenPort uint16, peerID string) (*Session, error) {
	session := Session{
		Peers:                   make([]*peer.Peer, 0),
		bundle:                  bnd,
		tf:                      tf,
		dbc:                     dbc,
		listenPort:              listenPort,
		peerID:                  peerID,
		maxDownloadRateKB:       0,
		maxUploadRateKB:         0,
		timeLastRequestsQueued:  time.Now(),
		timeLastResponsesQueued: time.Now(),
		maxRequestsOutRate:      0,
		maxResponsesOutRate:     0,
		maxPeers:                20,
		numPeers:                0,
		downloadRateKB:          0,
		uploadRateKB:            0,
		blockQueue:              make([]*peer.BlockRequest, 0),
		blockQueueMax:           50,
		running:                 false,
	}
	session.trackerList = trackerlist.New(
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
	session.running = true

	go session.run()

	return nil
}

func (session *Session) Stop() {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.running = false
	session.trackerList.Stop()
	for _, curPeer := range session.Peers {
		go curPeer.Close()
	}
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

// Internal

func (session *Session) run() {
	err := session.trackerList.Start()
	if err != nil {
		fmt.Printf("Error starting trackers: %v\n", err)
	}
	for session.running {
		if session.bundle.Complete { // Seeding
			return
			// Check for upload rate

			// Assign Responses

			// Check for new peers connecting
			//
		} else { // Leeching

			session.sortPeers()
			session.calcRates()

			session.checkCancelled()

			if debugopts.SESSION_DEBUG {
				fmt.Println("Session getting block requests from bundle...")
			}

			// Assign blocks to queue from bundle
			session.getBlocksFromBundle()

			if debugopts.SESSION_DEBUG {
				fmt.Printf("Session blockqueue length: %d\n", len(session.blockQueue))
			}

			session.assignParts()

			if debugopts.SESSION_DEBUG {
				fmt.Println("Session getting requests in...")
			}

			// Check for requests
			session.checkRequests()

			if debugopts.SESSION_DEBUG {
				fmt.Println("Session getting responses in...")
			}

			// Check for responses
			session.checkResponses()

			// Check for weird peers
			for _, curPeer := range session.Peers {
				if time.Now().After(curPeer.TimeConnected.Add(time.Second*20)) && curPeer.DownloadRateKB < 1 {
					session.disconnectPeer(curPeer.RemotePeerID)
				}
			}

			// Check for new peers connecting

			// Update Database
			session.dbc.UpdateBitfield(session.bundle.InfoHash, session.bundle.Bitfield)

			fmt.Print("Session bitfield: ")
			session.bundle.Bitfield.Print()

			// Check for new peers to connect to
			session.UpdatePeerInfo()
			if session.numPeers < session.maxPeers && session.numPeers < len(session.PeersInfo) {
				if debugopts.SESSION_DEBUG {
					fmt.Printf("Session checking for new peers(%d/%d)...\n", session.numPeers, session.maxPeers)
				}
				go session.findNewPeer()
			}

			if debugopts.SESSION_DEBUG {
				fmt.Printf("Session Rates: Down: %f KB/s Up: %f KB/s\n", session.downloadRateKB, session.uploadRateKB)
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (session *Session) UpdatePeerInfo() {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.PeersInfo = session.trackerList.GetPeers()
}

func (session *Session) getBlocksFromBundle() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if len(session.blockQueue) < session.blockQueueMax {
		space := session.blockQueueMax - len(session.blockQueue)
		blocksToRequest := session.bundle.NextNBlocks(space)
		requestsToAdd := make([]*peer.BlockRequest, 0)
		for _, block := range blocksToRequest {
			newReq := &peer.BlockRequest{
				Info:    *block,
				ReqTime: time.Time{},
				Sent:    false,
			}
			requestsToAdd = append(requestsToAdd, newReq)
		}
		session.blockQueue = append(session.blockQueue, requestsToAdd...)
	}
}

func (session *Session) checkRequests() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if session.maxUploadRateKB == 0 || session.uploadRateKB < session.maxUploadRateKB {
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
				nextResponse, err := session.bundle.GetBlock(&req.Info)
				if err != nil {
					fmt.Printf("err loading block: (%d, %d, %d) for response: %v\n", req.Info.PieceIndex, req.Info.BeginOffset, req.Info.Length, err)
					continue
				}
				curPeer.QueueResponseOut(&peer.BlockResponse{
					PieceIndex:  uint32(req.Info.PieceIndex),
					BeginOffset: uint32(req.Info.BeginOffset),
					Block:       nextResponse,
				})
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
				err := session.bundle.WriteBlock(
					int64(responses[0].PieceIndex),
					int64(responses[0].BeginOffset),
					responses[0].Block,
				)
				bi := bundle.BlockInfo{PieceIndex: int64(responses[0].PieceIndex), BeginOffset: int64(responses[0].BeginOffset), Length: int64(len(responses[0].Block))}
				if err != nil {
					fmt.Printf("error writing response (%d, %d) from peer(%s): %v\n", responses[0].PieceIndex, responses[0].BeginOffset, curPeer.RemotePeerID, err)
					session.bundle.CancelBlock(&bi)
				}
				i := 0
				req := &peer.BlockRequest{Info: bi, Sent: false, ReqTime: time.Time{}}
				for i < len(session.blockQueue) {
					if session.blockQueue[i].Equal(req) {
						session.blockQueue = append(session.blockQueue[:i], session.blockQueue[i+1:]...)
						break
					}
					i++
				}
				session.numBlocksSent--
				responses = responses[1:]
			}
		}
	}
}

func (session *Session) checkCancelled() {
	session.mux.Lock()
	defer session.mux.Unlock()

	if debugopts.SESSION_DEBUG {
		fmt.Println("Session checking for cancelled requests...")
	}

	// Check for cancelled pieces
	for _, curPeer := range session.Peers {
		if curPeer.NumRequestsCancelled() > 0 {
			cancelled := curPeer.GetCancelledRequests()
			if cancelled == nil {
				continue
			}
			for _, req := range cancelled {
				i := 0
				for i < len(session.blockQueue) {
					if req.Equal(session.blockQueue[i]) {
						session.bundle.CancelBlock(&req.Info)
						session.blockQueue = append(session.blockQueue[:i], session.blockQueue[i+1:]...)
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

	if debugopts.SESSION_DEBUG {
		fmt.Println("Session assigning requests to peers...")
	}

	if session.maxDownloadRateKB == 0 || session.downloadRateKB < session.maxDownloadRateKB {
		now := time.Now()
		timeSinceLastRequestsQueued := now.Sub(session.timeLastRequestsQueued)
		numBlocksCanQueue := int(float64(timeSinceLastRequestsQueued.Milliseconds()/1000) * session.maxRequestsOutRate)
		added := false
		// Assign requests to peers
		for peerIndex, curPeer := range session.Peers {
			if curPeer.RemoteChoking || curPeer.NumRequestsCanAdd() == 0 {
				continue
			}
			numCanAdd := curPeer.NumRequestsCanAdd()
			if session.maxDownloadRateKB != 0 {
				if numCanAdd > int(numBlocksCanQueue) {
					numCanAdd = int(numBlocksCanQueue)
				}
			}
			if numCanAdd > 0 {
				numAdded := 0
				curRequestIndex := 0
				for numAdded < numCanAdd && curRequestIndex < len(session.blockQueue) {
					curBlockReq := session.blockQueue[curRequestIndex]
					if !curBlockReq.Sent &&
						curPeer.HasPiece(curBlockReq.Info.PieceIndex) {
						session.numBlocksSent++
						curPeer.QueueRequestOut(curBlockReq.Info)
						session.blockQueue[curRequestIndex].Sent = true
						added = true
						numAdded++
					}
					curRequestIndex++
				}
			}
			if peerIndex == len(session.Peers) && session.numBlocksSent != len(session.blockQueue) {
				for _, curPeerInc := range session.Peers { // if not all blocks are being requested then double all block queues
					curPeerInc.SetMaxRequests(curPeerInc.RequestsOutMax * 2)
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

	if !session.bundle.Complete {
		blocksNeeded := session.blockQueueMax - len(session.blockQueue)
		addedBlocks := session.bundle.NextNBlocks(blocksNeeded)
		for _, bi := range addedBlocks {
			session.blockQueue = append(session.blockQueue, &peer.BlockRequest{Info: *bi, Sent: false, ReqTime: time.Time{}})
		}
	}
}

func (session *Session) findNewPeer() {
	if len(session.PeersInfo) == 0 {
		return
	}
	for _, peerInfo := range session.PeersInfo {
		found := false
		for _, conPeerInfo := range session.ConnectingPeers {
			if conPeerInfo.IP == peerInfo.IP && conPeerInfo.Port == peerInfo.Port {
				found = true
				break
			}
		}
		if found {
			continue
		}
		for _, curPeer := range session.Peers {
			if peerInfo.IP == curPeer.RemoteIP && peerInfo.Port == curPeer.RemotePort {
				found = true
				break
			}
		}
		if found {
			continue
		}
		session.mux.Lock()
		session.ConnectingPeers = append(session.ConnectingPeers, peerInfo)
		session.mux.Unlock()
		err := session.connectPeer(peerInfo)
		if err != nil {
			i := 0
			for i < len(session.ConnectingPeers) {
				if session.ConnectingPeers[i].IP == peerInfo.IP && session.ConnectingPeers[i].Port == peerInfo.Port {
					session.ConnectingPeers = append(session.ConnectingPeers[:i], session.ConnectingPeers[i+1:]...)
					break
				}
			}
			fmt.Printf("%v\n", err)
		} else {
			break
		}
	}
}

func (session *Session) connectPeer(pi *tracker.PeerInfo) error {
	session.mux.Lock()
	session.numPeers++
	session.mux.Unlock()

	for _, peer := range session.Peers {
		if peer.RemoteIP == pi.IP && peer.RemotePort == pi.Port {
			return errors.New("peer already connected")
		}
	}
	peer, err := peer.Connect(
		"",
		pi.IP,
		pi.Port,
		session.bundle.InfoHash,
		session.peerID,
		session.bundle.Bitfield,
		session.bundle.NumPieces,
	)
	if err != nil {
		session.mux.Lock()
		session.numPeers--
		session.mux.Unlock()
		return err
	}
	session.Peers = append(session.Peers, peer)
	return nil
}

func (session *Session) disconnectPeer(peerID string) error {
	session.mux.Lock()
	defer session.mux.Unlock()

	var peer *peer.Peer
	var peerIndex int
	for i, curPeer := range session.Peers {
		if curPeer.RemotePeerID == peerID {
			peer = curPeer
			peerIndex = i
		}
	}
	if peer == nil {
		return errors.New("ID not in peers list")
	}
	peer.Close()
	session.Peers = append(session.Peers[:peerIndex], session.Peers[peerIndex+1:]...)
	return nil
}

func (session *Session) sortPeers() {
	session.mux.Lock()
	defer session.mux.Unlock()

	sort.Slice(session.Peers, func(i, j int) bool {
		return session.Peers[i].DownloadRateKB < session.Peers[j].DownloadRateKB
	})
}

func (session *Session) calcRates() {
	session.mux.Lock()
	defer session.mux.Unlock()

	session.downloadRateKB = 0
	session.uploadRateKB = 0
	for _, peer := range session.Peers {
		session.downloadRateKB += peer.DownloadRateKB
		session.uploadRateKB += peer.UploadRateKB
	}
}
