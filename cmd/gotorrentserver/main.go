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
		break
	}
	if curPeer.Connected {
		time.Sleep(2 * time.Second)
		start := time.Now()
		nextBlocks := make([]*bundle.BlockInfo, 0)
		for !bdl.Complete && start.Add(3*time.Minute).After(time.Now()) {
			numBlocksNeeded := curPeer.NumRequestsCanAdd()
			nextBlocks = append(nextBlocks, bdl.NextNBlocks(numBlocksNeeded)...)
			fmt.Printf("Blocks to be requested (Added %d):\n", numBlocksNeeded)
			for _, block := range nextBlocks {
				fmt.Printf("(%d, %d, %d)\n", block.PieceIndex, block.BeginOffset, block.Length)
			}
			i := 0
			for i < len(nextBlocks) {
				err := curPeer.QueueRequestOut(*nextBlocks[i])
				if err != nil {
					fmt.Printf("Error adding request to queue: %v\n", err)
					i++
				} else {
					nextBlocks = append(nextBlocks[:i], nextBlocks[i+1:]...)
					fmt.Printf("Added request to queue...\n")
				}
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
			time.Sleep(time.Millisecond * 250)
			// Try write to bundle
		}
	} else {
		fmt.Println("Peer not connected")
	}
	curPeer.Close()
	time.Sleep(time.Second * 5)
}
