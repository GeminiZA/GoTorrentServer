package main

import (
	"fmt"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	fmt.Println("Parsing torrent file...")
	tf, err := torrentfile.ParseFile("test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Name: %s\nLength: %d\nPieceLength:%d\nInfoHash:%x\n", tf.Info.Name, tf.Info.Length, tf.Info.PieceLength, tf.InfoHash)
	fmt.Printf("Piece hash length: %d\n", len(tf.Info.Pieces[0]))
	fmt.Println("Creating Bundle...")
	fmt.Printf("Number of pieces: %d\n", len(tf.Info.Pieces))
	bundle, err := bundle.NewBundle(tf, "", 20)
	if err != nil {
		panic(err)
	}
	bundle.BitField.Print()
	fmt.Println("Bundle created")
	myPeerId := "-AZ2060-6wfG2wk6wWLc"
	
	tracker := tracker.New(tf.Announce, tf.InfoHash, 6881, 0, 0, bundle.BytesLeft(), myPeerId)
	err = tracker.Start()
	if err != nil {
		panic(err)
	}
	defer tracker.Stop()

	var curPeer *peer.Peer

	bi, err := bundle.NextBlock()
	if err != nil {
		panic(err)
	}

	bi.PieceIndex = 1

	foundPeer := false

	for _, peerDict := range tracker.Peers {
		peerIP, ok := peerDict["ip"].(string)
		if !ok {
			panic("peer ip not string")
		}
		peerPort, ok := peerDict["port"].(int64)
		if !ok {
			panic("peer port not int64")
		}
		peerID, ok := peerDict["peer id"].(string)
		if !ok {
			panic("peer id not string")
		}
		curPeer, err = peer.Connect(tf.InfoHash, bundle.NumPieces, peerIP, int(peerPort), peerID, myPeerId, bundle.BitField)
		if err != nil {
			fmt.Println(err)
			continue
		}

		i := 0
		for !curPeer.HasBitField() && i < 50 {
			i++
			time.Sleep(100 * time.Millisecond)
		}

		if i == 50 {
			curPeer.Close()
			continue
		}

		if curPeer.HasPiece(bi.PieceIndex) {
			foundPeer = true
			break
		}
	}

	if !foundPeer {
		panic("no peer found with piece")
	}

	defer curPeer.Close()

	fmt.Printf("Found peer(%s), with piece: %d\n", curPeer.PeerID, bi.PieceIndex)
	
	time.Sleep(time.Second)
	
	err = curPeer.DownloadBlock(bi)
	if err != nil {
		panic(err)
	}

	time.Sleep(30 * time.Second)
}