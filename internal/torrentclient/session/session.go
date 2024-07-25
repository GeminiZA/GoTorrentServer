package session

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
	"unicode"

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

	Peers            []*peer.Peer
	UnconnectedPeers []*tracker.PeerInfo
	ConnectedPeers   []*tracker.PeerInfo
	ConnectingPeers  []*tracker.PeerInfo

	listenPort uint16
	peerID     string
	maxPeers   int

	downloadRateKB float64
	uploadRateKB   float64

	BlockQueue    []*blockRequest
	BlockQueueMax int
	numBlocksSent int

	running bool
	mux     sync.Mutex
}

type blockRequest struct {
	Info     bundle.BlockInfo
	Sent     bool
	Rejected []string
	SentToID string
}

func (bra *blockRequest) Equal(brb *blockRequest) bool {
	return bra.Info.BeginOffset == brb.Info.BeginOffset &&
		bra.Info.PieceIndex == brb.Info.PieceIndex &&
		bra.Info.Length == brb.Info.Length
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
		maxPeers:                10,
		downloadRateKB:          0,
		uploadRateKB:            0,
		BlockQueue:              make([]*blockRequest, 0),
		BlockQueueMax:           50,
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
				fmt.Printf("Session blockqueue length: %d\n", len(session.BlockQueue))
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
				if time.Now().After(curPeer.TimeConnected.Add(time.Second*20)) && curPeer.TotalBytesDownloaded < 1 {
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
			if debugopts.SESSION_DEBUG {
				fmt.Printf("Current peers (max %d): \nConnected: %d\nConnecting: %d\nUnconnected: %d\n", session.maxPeers, len(session.ConnectedPeers), len(session.ConnectingPeers), len(session.UnconnectedPeers))
			}
			if len(session.ConnectedPeers)+len(session.ConnectingPeers) < session.maxPeers && len(session.UnconnectedPeers) > 0 {
				go session.findNewPeer()
			}

			if debugopts.SESSION_DEBUG {
				fmt.Printf("Session Rates: Down: %f KB/s Up: %f KB/s\n", session.downloadRateKB, session.uploadRateKB)
			}

			if debugopts.SESSION_DEBUG_simple {
				fmt.Printf("Session:\nDownloadRate: %f kbps\nUploadRate: %f kbps\nQueueLength: %d\nPeers:\n\tID\t\t\tIP\t\tPort\tDownloaded\tUploaded\n", session.downloadRateKB, session.uploadRateKB, len(session.BlockQueue))
				for _, curPeer := range session.Peers {
					fmt.Printf("\t%s\t%s\t%d\t%d\t\t%d\n", sanitize(curPeer.RemotePeerID), curPeer.RemoteIP, curPeer.RemotePort, curPeer.TotalBytesDownloaded, curPeer.TotalBytesUploaded)
				}
			}

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
	newPeers := session.trackerList.GetPeers()

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
		blocksToRequest := session.bundle.NextNBlocks(space)
		requestsToAdd := make([]*blockRequest, 0)
		for _, block := range blocksToRequest {
			newReq := &blockRequest{
				Info:     *block,
				Rejected: make([]string, 0),
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
				req := &blockRequest{Info: bi, Sent: false}
				for i < len(session.BlockQueue) {
					if session.BlockQueue[i].Equal(req) {
						session.BlockQueue = append(session.BlockQueue[:i], session.BlockQueue[i+1:]...)
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
				for i < len(session.BlockQueue) {
					if req.Info.PieceIndex == session.BlockQueue[i].Info.PieceIndex &&
						req.Info.BeginOffset == session.BlockQueue[i].Info.BeginOffset &&
						req.Info.Length == session.BlockQueue[i].Info.Length {
						session.BlockQueue[i].Rejected = append(session.BlockQueue[i].Rejected, curPeer.RemotePeerID)
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
			if numCanAdd == curPeer.RequestsOutMax {
				curPeer.SetMaxRequests(curPeer.RequestsOutMax * 2)
			}
			if session.maxDownloadRateKB != 0 {
				if numCanAdd > int(numBlocksCanQueue) {
					numCanAdd = int(numBlocksCanQueue)
				}
			}
			if numCanAdd > 0 {
				numAdded := 0
				curRequestIndex := 0
				for numAdded < numCanAdd && curRequestIndex < len(session.BlockQueue) {
					curBlockReq := session.BlockQueue[curRequestIndex]
					rejected := false
					for _, rej := range curBlockReq.Rejected {
						if rej == curPeer.RemotePeerID {
							rejected = true
							break
						}
					}
					if !curBlockReq.Sent &&
						!rejected &&
						curPeer.HasPiece(curBlockReq.Info.PieceIndex) {
						session.numBlocksSent++
						curPeer.QueueRequestOut(curBlockReq.Info)
						session.BlockQueue[curRequestIndex].Sent = true
						session.BlockQueue[curRequestIndex].SentToID = curPeer.RemotePeerID
						added = true
						numAdded++
					}
					if rejected && peerIndex == len(session.Peers) {
						session.BlockQueue[curRequestIndex].Rejected = make([]string, 0)
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

	if !session.bundle.Complete {
		blocksNeeded := session.BlockQueueMax - len(session.BlockQueue)
		addedBlocks := session.bundle.NextNBlocks(blocksNeeded)
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
		if debugopts.SESSION_DEBUG {
			fmt.Printf("session trying to connect to peer(%s:%d)...\n", peerInfo.IP, peerInfo.Port)
		}
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
			fmt.Printf("Session error connecting to peer: %v\n", err)
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

func (session *Session) connectPeer(pi *tracker.PeerInfo) error {
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
	i := 0
	for i < len(session.BlockQueue) {
		if session.BlockQueue[i].Sent && session.BlockQueue[i].SentToID == peerID {
			session.bundle.CancelBlock(&session.BlockQueue[i].Info)
			session.BlockQueue = append(session.BlockQueue[:i], session.BlockQueue[i+1:]...)
		} else {
			i++
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

	session.downloadRateKB = 0
	session.uploadRateKB = 0
	for _, peer := range session.Peers {
		session.downloadRateKB += peer.DownloadRateKB
		session.uploadRateKB += peer.UploadRateKB
	}
}
