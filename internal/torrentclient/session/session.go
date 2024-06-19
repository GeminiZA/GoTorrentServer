package session

import (
	"errors"
	"fmt"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

const DEBUG_SESSION bool = true

type BlockReq struct {
	info  			*bundle.BlockInfo
	fetching 		bool
	fetched 		bool
	failed 			bool
	timeRequested   time.Time
}

type Session struct {
	tf 				 *torrentfile.TorrentFile
	peers 			 []*peer.Peer
	curPeerIDs		 map[string]struct{}
	bundle 			 *bundle.Bundle
	tracker 		 *tracker.Tracker
	myPeerID		 string
	outport			 int
	maxConnections   int
	curConnections   int
	running 		 bool
	pendingBlocks 	 []BlockReq
	maxPendingBlocks int
	seeding 		 bool
	peerConnectChan  chan *peer.Peer
}

func New(torrentFile *torrentfile.TorrentFile, bundle *bundle.Bundle, maxCon int, maxPendingBlocks int, myPeerID string, outport int) (*Session, error) {
	session := Session{
		tf: torrentFile,
		peers: make([]*peer.Peer, 0), 
		curPeerIDs: make(map[string]struct{}),
		bundle: bundle, 
		myPeerID: myPeerID,
		outport: outport,
		maxConnections: maxCon, 
		curConnections: 0,
		maxPendingBlocks: maxPendingBlocks,
		pendingBlocks: make([]BlockReq, 0),
		seeding: false,
		peerConnectChan: make(chan *peer.Peer, maxCon),
	}
	return &session, nil
}

func (session *Session) ConnectPeer(peerIP string, peerPort int, peerID string) {
	peer, err := peer.Connect(
		session.tf.InfoHash,
		session.bundle.NumPieces,
		peerIP,
		peerPort,
		peerID,
		session.myPeerID,
		session.bundle.BitField,
		make(chan *message.Message, 10),
		make(chan *message.Message, 10),
	)
	if err != nil {
		session.curConnections--
		delete(session.curPeerIDs, peerID)
		fmt.Println("Error connecting to peer: ", err)
		return
	}
	peer.Seeding = session.seeding
	session.peerConnectChan<-peer
	if DEBUG_SESSION {
		fmt.Printf("Session added peer to channel: %s\n", peer.PeerID)
	}
}

func (session *Session) Start() error {
	if session.tf == nil || session.peers == nil || session.bundle == nil {
		return errors.New("session not instantiated properly")
	}
	session.tracker = tracker.New(session.tf.Announce, session.tf.InfoHash, session.outport, 0, 0, session.bundle.BytesLeft(), session.myPeerID)
	err := session.tracker.Start()
	if err != nil {
		return err
	}
	session.seeding = session.bundle.Complete
	session.running = true
	go session.runSession()
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
				if DEBUG_SESSION {
					fmt.Printf("Added block req to %s\n", peer.PeerID)
				}
				peer.DownloadBlock(bi)
				break
			}
		}
	}
}

func (session *Session) PrintPeers() {
	for _, peer := range session.peers {
		fmt.Printf("Peer (%s)\tConnected: %t\tIP: %s\tPort:%d\n", peer.PeerID, peer.IsConnected(), peer.IP, peer.Port)
	}
}


func (session *Session) runSession() {
	for session.running {
		// Handle new peer connections
		select {
		case peer := <-session.peerConnectChan:
			session.peers = append(session.peers, peer)
			fmt.Printf("Session added peer: %s\n", peer.PeerID)
		default:
			//No peers to add
		}

		// Add new peer connections
		if session.curConnections < session.maxConnections {
			fmt.Println("Looking for more peers...\nCurrent Peers:")
			session.PrintPeers()
			for _, peerDict := range session.tracker.Peers {
				if session.curConnections == session.maxConnections {
					break
				}
				peerID, _ := peerDict["peer id"].(string)
				if _, ok := session.curPeerIDs[peerID]; ok {
					continue
				}
				session.curPeerIDs[peerID] = struct{}{}
				session.curConnections++
				peerIP, _ := peerDict["ip"].(string)
				peerPort, _ := peerDict["port"].(int64)
				go session.ConnectPeer(peerIP, int(peerPort), peerID)
			}
		}
		//Handle disconnected peers
		for i := range session.peers {
			if !session.peers[i].IsConnected() {
				session.peers[i] = session.peers[len(session.peers)]
				session.peers = session.peers[:len(session.peers) - 1]
			}
		}

		// Set if seeding
		if session.bundle.Complete {
			session.seeding = true
		}

		//Handle already fetched block requests
		i := 0
		for i < len(session.pendingBlocks) {
			//req timed out
			if session.pendingBlocks[i].failed {
				if DEBUG_SESSION {
					fmt.Printf("Request block (%d, %d) failed\n", session.pendingBlocks[i].info.PieceIndex, session.pendingBlocks[i].info.BeginOffset)
				}
				session.bundle.CancelBlock(session.pendingBlocks[i].info)
				session.pendingBlocks[i] = session.pendingBlocks[len(session.pendingBlocks) - 1]
				session.pendingBlocks = session.pendingBlocks[:len(session.pendingBlocks) - 1]
			} else {
				//req successful
				if session.pendingBlocks[i].fetched {
					if DEBUG_SESSION {
						fmt.Printf("Request block (%d, %d) successful\n", session.pendingBlocks[i].info.PieceIndex, session.pendingBlocks[i].info.BeginOffset)
					}
					session.pendingBlocks[i] = session.pendingBlocks[len(session.pendingBlocks) - 1]
					session.pendingBlocks = session.pendingBlocks[:len(session.pendingBlocks) - 1]
				} else {
					i++
				}
			}
		}

		// Add new block requests
		for len(session.pendingBlocks) < session.maxPendingBlocks {
			bi, err := session.bundle.NextBlock()
			if err != nil {
				fmt.Println("runsession error: ", err)
				continue
			}
			if DEBUG_SESSION {
				fmt.Printf("Added block: PieceIndex: %d, Offset: %d, Length: %d to pendingBlocks\n", bi.PieceIndex, bi.BeginOffset, bi.Length)
			}
			session.pendingBlocks = append(session.pendingBlocks, BlockReq{info: bi, fetching: false, fetched: false, failed: false, timeRequested: time.Now()})
		}

		// Handle peer messages
		for _, peer := range session.peers {
			// Send requests to peer
			if peer.IsConnected() {
				for i, br := range session.pendingBlocks {
					//if DEBUG_SESSION {
						//fmt.Printf("Checking for req[%d]: Fetching: %t, Failed: %t, Fetched: %t\n", i, br.fetching, br.failed, br.fetched)
					//}
					if session.pendingBlocks[i].fetching {
						if time.Now().After(session.pendingBlocks[i].timeRequested.Add(10 * time.Second)) {
							session.pendingBlocks[i].failed = true
						}
						continue
					}
					if DEBUG_SESSION {
						fmt.Printf("Added block req (Piece: %d, offSet: %d) to %s\n", br.info.PieceIndex, br.info.BeginOffset, peer.PeerID)
					}
					peer.DownloadBlock(session.pendingBlocks[i].info)
					session.pendingBlocks[i].fetching = true
					break
				}
			}

			//Process pieces and requests from peer
			select {
			case msg := <-peer.MsgInChan:
				if DEBUG_SESSION {
					fmt.Print("Session Got message: ")
					msg.Print()
				}
				switch msg.Type {
				case message.PIECE:
					found := false
					for i, br := range session.pendingBlocks {
						if !br.fetching || br.failed {
							continue
						}
						//Check if block is in pending requests before writing
						if msg.Index == uint32(br.info.PieceIndex) && msg.Begin == uint32(br.info.BeginOffset) && uint32(len(msg.Piece)) == uint32(br.info.Length) {
							found = true
							err := session.bundle.WriteBlock(int64(msg.Index), int64(msg.Begin), msg.Piece)
							if err != nil {
								fmt.Println("Error writing block, ", err)
								break
							}
							session.pendingBlocks[i].fetching = false
							session.pendingBlocks[i].fetched = true
						}
					}
					if DEBUG_SESSION && !found {
						fmt.Printf("Piece message not in pending: ")
						msg.Print()
						fmt.Println("Pending:")
						for _, br := range session.pendingBlocks {
							fmt.Printf("Piece Index: %d, Offset: %d, Length: %d, Fetching: %t, Fetched: %t, Failed: %t\n", br.info.PieceIndex, br.info.BeginOffset, br.info.Length, br.fetching, br.fetched, br.failed)
						}
					}
				case message.REQUEST:
					// Handle sending block to peer
					if session.bundle.BitField.GetBit(int64(msg.Index)) {
						bi := &bundle.BlockInfo{PieceIndex: int64(msg.Index), BeginOffset: int64(msg.Begin), Length: int64(msg.Length)}
						block, err := session.bundle.GetBlock(bi)
						if err != nil {
							fmt.Println("Error getting block, ", err)
						} else {
							peer.MsgOutChan <- message.NewPiece(msg.Index, msg.Begin, block)
						}
					}
				}
			default:
				// No messages wait 10 ms
				time.Sleep(10 * time.Millisecond)
			}

		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (session *Session) PrintStatus() {
	session.bundle.PrintStatus()
	fmt.Println("Peers:")
	for _, peer := range session.peers {
		fmt.Printf("Peer (%s)\tConnected: %t\tIP: %s\tPieces: %d\n", peer.PeerID, peer.IsConnected(), peer.IP, peer.NumPieces())
	}
}

func (session *Session) Close() {
	session.running = false
	for _, peer := range session.peers {
		peer.Close()
	}
	session.tracker.Stop()
}