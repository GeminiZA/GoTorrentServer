package session

import (
	"errors"
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

const DEBUG_SESSION bool = true

type Session struct {
	tf 				 *torrentfile.TorrentFile
	peers 			 []*peer.Peer
	curPeerIDs		 map[string]struct{}
	bannedPeerIDs	 map[string]struct{}
	bundle 			 *bundle.Bundle
	interestedBits	 *bitfield.BitField
	tracker 		 *tracker.Tracker
	myPeerID		 string
	outport			 int
	maxConnections   int
	curConnections   int
	running 		 bool
	maxPendingPieces int
	seeding 		 bool
	peerConnectChan  chan *peer.Peer
}

func New(torrentFile *torrentfile.TorrentFile, bundle *bundle.Bundle, maxCon int, maxPendingPieces int, myPeerID string, outport int) (*Session, error) {
	session := Session{
		tf: torrentFile,
		peers: make([]*peer.Peer, 0), 
		curPeerIDs: make(map[string]struct{}),
		bundle: bundle, 
		myPeerID: myPeerID,
		outport: outport,
		maxConnections: maxCon, 
		curConnections: 0,
		maxPendingPieces: maxPendingPieces,
		seeding: false,
		peerConnectChan: make(chan *peer.Peer, maxCon),
	}
	session.interestedBits = bitfield.New(session.bundle.NumPieces)
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

func (session *Session) PrintPeers() {
	for _, peer := range session.peers {
		fmt.Printf("Peer (%s)\tConnected: %t\tIP: %s\tPort:%d\n", peer.PeerID, peer.IsConnected, peer.IP, peer.Port)
	}
}


func (session *Session) runSession() {
	for session.running {

		// Update interested pieces
		session.interestedBits = session.bundle.BitField.Not()

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
				if _, ok := session.bannedPeerIDs[peerID]; ok {
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
			if !session.peers[i].IsConnected {
				session.peers[i] = session.peers[len(session.peers)]
				session.peers = session.peers[:len(session.peers) - 1]
			}
		}

		// Set if seeding
		if session.bundle.Complete {
			session.seeding = true
		}

		for _, curPeer := range session.peers {
			//Handle messages in
			select {
			case curReplyIn := <-curPeer.ReplyInChan:
				//check if I needed that reply
				if session.bundle.BitField.GetBit(curReplyIn.Index) {
					continue
				}
				if session.bundle.Pieces[curReplyIn.Index].IsBlockWritten(curReplyIn.Offset) {
					continue
				}
				err := session.bundle.WriteBlock(curReplyIn.Index, curReplyIn.Offset, curReplyIn.Bytes)
				if err != nil {
					fmt.Println("Error writing block: ", err)
					continue
				}
			default:
				//Do nothing (no replies in)
			}
			select {
			case curRequestIn := <-curPeer.RequestInChan:
				if !session.bundle.BitField.GetBit(curRequestIn.Info.PieceIndex) {
					continue
				}
				block, err := session.bundle.GetBlock(curRequestIn.Info)
				if err != nil {
					fmt.Println("Error getting block from bundle: ", err)
					continue
				}
				reply := peer.BlockReply{Index: curRequestIn.Info.PieceIndex, Offset: curRequestIn.Info.BeginOffset, Bytes: block }
				curPeer.ReplyOutChan <- reply
			default:
				//Do nothing (no requests)
			}
		}
		}

}

func (session *Session) PrintStatus() {
	session.bundle.PrintStatus()
	fmt.Println("Peers:")
	for _, peer := range session.peers {
		fmt.Printf("Peer (%s)\tConnected: %t\tIP: %s\tPieces: %d\n", peer.PeerID, peer.IsConnected, peer.IP, peer.NumPieces())
	}
}

func (session *Session) Close() {
	session.running = false
	for _, peer := range session.peers {
		peer.Close()
	}
	session.tracker.Stop()
}