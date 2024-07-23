package session

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type Session struct {
	bundle  *bundle.Bundle
	dbc     *database.DBConn
	tracker *tracker.Tracker
	tf      *torrentfile.TorrentFile

	maxDownloadRateKB float64
	maxUploadRateKB   float64

	peers []*peer.Peer

	listenPort int
	peerID     string
	maxPeers   int

	downloadRateKB float64
	uploadRateKB   float64

	blockQueue    []*peer.BlockRequest
	blockQueueMax int
	numBlocksSent int

	running bool
}

// Exported

func New(bnd *bundle.Bundle, dbc *database.DBConn, tf *torrentfile.TorrentFile, listenPort int, peerID string) (*Session, error) {
	session := Session{
		peers:             make([]*peer.Peer, 0),
		bundle:            bnd,
		tf:                tf,
		dbc:               dbc,
		listenPort:        listenPort,
		peerID:            peerID,
		maxDownloadRateKB: 0,
		maxUploadRateKB:   0,
		maxPeers:          1,
		downloadRateKB:    0,
		uploadRateKB:      0,
		blockQueue:        make([]*peer.BlockRequest, 0),
		blockQueueMax:     50,
		running:           false,
	}
	session.tracker = tracker.New(tf.AnnounceList, tf.InfoHash, listenPort, 0, 0, 0, peerID)
	return &session, nil
}

func (session *Session) Start() error {
	err := session.tracker.Start()
	if err != nil {
		return fmt.Errorf("error starting tracker in session: %v", err)
	}
	session.running = true

	go session.run()

	return nil
}

func (session *Session) Stop() {
	session.running = false
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
	session.maxDownloadRateKB = rateKB
}

func (session *Session) SetMaxUpRate(rateKB float64) {
	session.maxUploadRateKB = rateKB
}

// Internal

func (session *Session) run() {
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

			// Check for cancelled pieces
			for _, curPeer := range session.peers {
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

			// Assign blocks to queue
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

			if session.maxDownloadRateKB == 0 || session.downloadRateKB < session.maxDownloadRateKB {
				// Assign requests to peers
				for peerIndex, curPeer := range session.peers {
					if curPeer.RemoteChoking {
						continue
					}
					numCanAdd := curPeer.NumRequestsCanAdd()
					if numCanAdd > 0 {
						numAdded := 0
						curRequestIndex := 0
						for numAdded < numCanAdd && curRequestIndex < len(session.blockQueue) {
							if !session.blockQueue[curRequestIndex].Sent {
								session.numBlocksSent++
								curPeer.QueueRequestOut(session.blockQueue[curRequestIndex].Info)
								session.blockQueue[curRequestIndex].Sent = true
								numAdded++
							}
							curRequestIndex++
						}
					}
					if peerIndex == len(session.peers) && session.numBlocksSent != len(session.blockQueue) {
						for _, curPeerInc := range session.peers { // if not all blocks are being requested then double all block queues
							curPeerInc.SetMaxRequests(curPeerInc.RequestOutMax * 2)
						}
					}
				}
			}

			// Check for requests
			if session.maxUploadRateKB == 0 || session.uploadRateKB < session.maxUploadRateKB {
				for _, curPeer := range session.peers {
					requests := curPeer.GetRequestsIn()
					responses := make([]*peer.BlockResponse, 0)
					for _, req := range requests {
						if curPeer.NumResponsesOut() == curPeer.ResponseOutMax {
							break
						}
						nextResponse, err := session.bundle.GetBlock(&req.Info)
						if err != nil {
							fmt.Printf("err loading block: (%d, %d, %d) for response: %v\n", req.Info.PieceIndex, req.Info.BeginOffset, req.Info.Length, err)
							continue
						}
						responses = append(responses, &peer.BlockResponse{
							PieceIndex:  uint32(req.Info.PieceIndex),
							BeginOffset: uint32(req.Info.BeginOffset),
							Block:       nextResponse,
						})
					}
				}
			}

			// Check for responses
			for _, curPeer := range session.peers {
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
			// Check for weird peers
			for _, curPeer := range session.peers {
				if time.Now().After(curPeer.TimeConnected.Add(time.Second*20)) && curPeer.DownloadRateKB < 1 {
					session.disconnectPeer(curPeer.RemotePeerID)
				}
			}

			// Check for new peers connecting

			// Update Database
			session.dbc.UpdateBitfield(session.bundle.InfoHash, session.bundle.Bitfield)

			// Check for new peers to connect to
			if len(session.peers) < session.maxPeers {
				session.findNewPeer()
			}
		}
	}
}

func (session *Session) findNewPeer() {
	for _, peerInfo := range session.tracker.Peers {
		session.connectPeer(peerInfo)
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
			session.blockQueue = append(session.blockQueue, &peer.BlockRequest{Info: *bi, Sent: false, ReqTime: time.Time{}})
		}
	}
}

func (session *Session) connectPeer(pi tracker.PeerInfo) error {
	for _, peer := range session.peers {
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
		err = fmt.Errorf("error connecting to peer: %s", err)
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
