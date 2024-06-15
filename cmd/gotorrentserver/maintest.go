package main

import (
	"fmt"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer/message"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	fmt.Println("Parsing torrent file...")
	tf, err := torrentfile.ParseFile("test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Name: %s\nLength: %d\nPieceLength:%d\n", tf.Info.Name, tf.Info.Length, tf.Info.PieceLength)
	fmt.Printf("Piece hash length: %d\n", len(tf.Info.Pieces[0]))
	fmt.Println("Creating Bundle...")
	bundle, err := bundle.NewBundle(tf, "", 20)
	if err != nil {
		panic(err)
	}
	bundle.BitField.Print()
	fmt.Println("Bundle created")
	tracker := tracker.New(tf.Announce, tf.InfoHash, 6881, 0, 0, bundle.Length, "-AZ2060-6wfG2wk6wWLc")
	tracker.Start()
	defer tracker.Stop()
	fmt.Println(tracker.Peers)

	for _, peerDict := range tracker.Peers {
		var peerIP string
		var peerPort int64
		var ok bool
		if peerIP, ok = peerDict["ip"].(string); !ok {
			fmt.Println("Peer ip not string")
			continue
		}
		if peerPort, ok = peerDict["port"].(int64); !ok {
			fmt.Println("Peer port not int")
			continue
		}
		fmt.Printf("Trying to connect to peer (%s:%d)...\n", peerIP, peerPort)
		msgChan := make(chan *message.Message, 100)
		peer, err := peer.Connect(tf.InfoHash, bundle.NumPieces, peerIP, int(peerPort), tracker.PeerID, msgChan)
		defer peer.Close()
		if err != nil {
			fmt.Println("error connecting to peer")
			fmt.Println(err)
			continue
		}
		fmt.Printf("Connected to peer: %s\n", peer.PeerID)
		err = peer.SendInterested()
		if err != nil {
			panic(err)
		}
		//time.Sleep(2 * time.Second)
		//err = peer.SendUnchoke()
		//if err != nil {
			//panic(err)
		//}
		// FOr some reason getting piece response now when not waiting between messages
			//send request
			for !bundle.Complete {
				pieceIndex, beginOffset, length, err := bundle.NextBlock()
				if err != nil {
					panic(err)
				}
				waitingForBlock := false
				if !peer.PeerChoking {
					if !waitingForBlock {
						fmt.Printf("Sending REQUEST: piece: %d, offset: %d, length: %d\n", pieceIndex, beginOffset, length)
						err = peer.SendRequestBlock(pieceIndex, beginOffset, length)
						if err != nil {
							panic(err)
						}
						waitingForBlock = true
					}
					for waitingForBlock {
						curMsg := <-msgChan
						if curMsg.Type == message.PIECE {
							err := bundle.WriteBlock(int64(curMsg.Index), int64(curMsg.Begin), curMsg.Piece)
							if err != nil {
								panic(err)
							}
							waitingForBlock = false
						}
						curMsg = nil
					}
				} else {
					fmt.Println("Peer still choking...")
				}
				time.Sleep(100 * time.Millisecond)
			}
	}
}