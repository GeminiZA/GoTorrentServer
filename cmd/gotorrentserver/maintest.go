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
		if err != nil {
			fmt.Println("error connecting to peer")
			fmt.Println(err)
			continue
		}
		defer peer.Close()
		fmt.Printf("Connected to peer: %s\n", peer.PeerID)
		err = peer.SendInterested()
		if err != nil {
			panic(err)
		}
		peer.PeerChoking = true
		for peer.PeerChoking {
			fmt.Println("Peer choking...")
			time.Sleep(100 * time.Millisecond)
		}
		for !bundle.Complete {
			waitingForPiece := false
			if !peer.PeerChoking {
				if !waitingForPiece {
					pieceIndex, beginOffset, length, err := bundle.NextBlock()
					if err != nil {
						panic(err)
					}
					err = peer.SendRequestBlock(pieceIndex, beginOffset, length)
					if err != nil {
						panic(err)
					}
					waitingForPiece = true
				}
				msg := <-msgChan
				fmt.Println("Message got in main goroutine:")
				msg.Print()
				if msg.Type == message.PIECE {
					fmt.Println("GOT PIECE !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
				}
			} else {
				fmt.Println("Peer choking...")
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}