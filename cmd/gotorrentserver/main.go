package main

import (
	"fmt"
	"os"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	args := os.Args
	fmt.Println(args)

	if len(os.Args) > 2 {
		arg := os.Args[2]
		if arg == "test" {
			if len(os.Args) < 4 {
				panic("no package to test specified")
			}
			testPkg := os.Args[3]
			if testPkg == "peer" {
				// Peer test
				if len(os.Args) < 5 {
					panic("in peer test no torrent file specified")
				}
				TestPeer(os.Args[4], "TESTID12321232123212")
			} else if testPkg == "session" {
				// Session tests
			} else {
				panic(fmt.Errorf("pkg: %s tests not implemented", testPkg))
			}
		} else {
			fmt.Println("Server not implemented")
			// Run server
		}
	}
}

func TestPeer(tfPath string, myPeerID string) {
	// Connect to a peer
	tf, err := torrentfile.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	bdl, err := bundle.NewBundle(tf, "./TestBundle", 20)
	if err != nil {
		panic(err)
	}
	trk := tracker.New(
		tf.Announce,
		bdl.InfoHash,
		6681,
		0,
		0,
		bdl.Length,
		myPeerID,
	)
	err = trk.Start()
	if err != nil {
		fmt.Printf("Error starting tracker: %v\n", err)
	}
	defer trk.Stop()
	var curPeer *peer.Peer
	for _, peerDetails := range trk.Peers {
		if peerDetails.PeerID == myPeerID {
			continue
		}
		fmt.Printf("Trying to connect to peer(%s)...\n", peerDetails.PeerID)
		curPeer, err = peer.Connect(
			peerDetails.PeerID,
			peerDetails.IP,
			peerDetails.Port,
			bdl.InfoHash,
			myPeerID,
			bdl.Bitfield,
			bdl.NumPieces,
		)
		if err != nil {
			fmt.Printf("Error connecting to peer(%s): %v\n", peerDetails.PeerID, err)
			continue
		}
		fmt.Printf("Succesfully connected to peer (%s)\n", peerDetails.PeerID)
		defer curPeer.Close()
		break
	}
	if curPeer.Connected {
		time.Sleep(2 * time.Second)
		start := time.Now()
		for start.Add(10 * time.Second).After(time.Now()) {
			numBlocksNeeded := curPeer.NumRequestsCanAdd()
			nextBlocks := bdl.NextNBlocks(numBlocksNeeded)
			fmt.Println("Blocks to be requested:")
			for _, block := range nextBlocks {
				fmt.Printf("(%d, %d, %d)\n", block.PieceIndex, block.BeginOffset, block.Length)
			}
			for _, blockinfo := range nextBlocks {
				fmt.Printf("Queuing request: (%d, %d, %d)\n", blockinfo.PieceIndex, blockinfo.BeginOffset, blockinfo.Length)
				curPeer.QueueRequestOut(*blockinfo)
			}
			curResp := curPeer.GetResponseIn()
			for curResp != nil {
				fmt.Printf("Got response in main: Index: %d, Offset: %d\n", curResp.PieceIndex, curResp.BeginOffset)
				err = bdl.WriteBlock(int64(curResp.PieceIndex), int64(curResp.BeginOffset), curResp.Block)
				if err != nil {
					fmt.Printf("Error writing block (%d, %d): %v", curResp.PieceIndex, curResp.BeginOffset, err)
				}
				curResp = curPeer.GetResponseIn()
			}
			time.Sleep(time.Millisecond * 500)
			// Try write to bundle
		}
	} else {
		fmt.Println("Peer not connected")
	}
}
