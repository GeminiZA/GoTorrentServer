package main

import (
	"fmt"
	"os"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	args := os.Args
	fmt.Println(args)

	if len(os.Args) > 2 {
		arg := os.Args[2]
		if arg == "test" {
			myPeerID := "-TR2940-6oFw2M6BdUkY"
			if len(os.Args) < 4 {
				panic("no package to test specified")
			}
			testPkg := os.Args[3]
			if testPkg == "peer" {
				// Peer test
				if len(os.Args) < 5 {
					panic("in peer test no torrent file specified")
				}
				TestPeer(os.Args[4], myPeerID)
			} else if testPkg == "session" {
				if len(os.Args) < 5 {
					panic("in peer test no torrent file specified")
				}
				TestSession(os.Args[4], myPeerID)
			} else {
				panic(fmt.Errorf("pkg: %s tests not implemented", testPkg))
			}
		} else {
			fmt.Println("Server not implemented")
			// Run server
		}
	}
}

func TestSession(tfPath string, myPeerID string) {
	tf, err := torrentfile.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	bdl, err := bundle.NewBundle(tf, "./TestBundle", 20)
	if err != nil {
		panic(err)
	}
	dbc, err := database.Connect()
	if err != nil {
		panic(err)
	}
	defer dbc.Disconnect()
	listenPort := 6681
	sesh, err := session.New(bdl, dbc, tf, listenPort, myPeerID)
	if err != nil {
		panic(err)
	}
	err = sesh.Start()
	if err != nil {
		panic(err)
	}
	defer sesh.Stop()
	start := time.Now()
	for start.Add(time.Second*60).After(time.Now()) && !bdl.Complete {
		time.Sleep(time.Second)
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
		tf.AnnounceList,
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
		return
	}
	defer trk.Stop()
	var curPeer *peer.Peer
	for _, peerDetails := range trk.Peers {
		fmt.Printf("Trying to connect to peer(%s)...\n", peerDetails.IP)
		curPeer, err = peer.Connect(
			"",
			peerDetails.IP,
			peerDetails.Port,
			bdl.InfoHash,
			myPeerID,
			bdl.Bitfield,
			bdl.NumPieces,
		)
		if err != nil {
			fmt.Printf("Error connecting to peer(%s): %v\n", peerDetails.IP, err)
			continue
		}
		fmt.Printf("Succesfully connected to peer (%s)\n", peerDetails.IP)
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
			if curPeer.NumResponsesIn() > 0 {
				responses := curPeer.GetResponsesIn()
				for len(responses) > 0 {
					err := bdl.WriteBlock(int64(responses[0].PieceIndex), int64(responses[0].BeginOffset), responses[0].Block)
					if err != nil {
						fmt.Printf("err writing block: %v\n", err)
					}
					responses = responses[1:]
				}
			}
			time.Sleep(time.Millisecond * 250)
			// Try write to bundle
		}
	} else {
		fmt.Println("Peer not connected")
	}
	fmt.Printf("Peer test complete:\nDownloadRate: %f kbps\nUploadRate: %f kbps\n", curPeer.DownloadRateKB, curPeer.UploadRateKB)
	curPeer.Close()
}
