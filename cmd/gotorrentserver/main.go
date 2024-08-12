package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/client"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/trackerlist"
)

func main() {
	args := os.Args
	fmt.Println(args)

	if len(os.Args) > 2 {
		arg := os.Args[2]
		if arg == "test" {
			logger := log.Default()
			myPeerID := "-TR2940-6oFw2M6BdUkY"
			if len(os.Args) < 4 {
				panic("no package to test specified")
			}
			testPkg := os.Args[3]
			switch testPkg {
			case "peer":
				// Peer test
				if len(os.Args) < 5 {
					panic("in peer test no torrent file specified")
				}
				fmt.Printf("Testing peer with file: %s\n", os.Args[4])
				TestPeer(os.Args[4], myPeerID)
			case "session":
				if len(os.Args) < 5 {
					panic("in peer test no torrent file specified")
				}
				TestSession(os.Args[4], myPeerID)
			case "tracker":
				TestTrackerList(os.Args[4], myPeerID, logger)
			case "client":
				TestClient(os.Args[4])
			default:
				panic(fmt.Errorf("pkg: %s tests not implemented", testPkg))
			}

		} else {
			fmt.Println("Server not implemented")
			// Run server
		}
	}
}

func TestClient(tfPath string) {
	tc, err := client.Start()
	defer tc.Stop()
	if err != nil {
		fmt.Println(err)
		return
	}
	time.Sleep(5 * time.Second)
}

func TestTrackerList(tfPath string, myPeerID string, logger *log.Logger) {
	tf := torrentfile.New()
	err := tf.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	bdl, err := bundle.Create(tf, "./TestBundle")
	if err != nil {
		panic(err)
	}
	dbc, err := database.Connect()
	if err != nil {
		panic(err)
	}
	defer dbc.Disconnect()
	listenPort := uint16(6681)
	tl := trackerlist.New(tf.Announce, tf.AnnounceList, bdl.InfoHash, listenPort, bdl.Length, false, myPeerID)
	err = tl.Start()
	if err != nil {
		panic(err)
	}
	time.Sleep(10 * time.Second)
	tl.Stop()
}

func TestSession(tfPath string, myPeerID string) {
	tf := torrentfile.New()
	err := tf.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	listenPort := uint16(6681)
	sesh, err := session.New("./TestBundle", tf, bitfield.New(int64(len(tf.Info.Pieces))), listenPort, myPeerID)
	if err != nil {
		panic(err)
	}
	// sesh.SetMaxDownloadRate(512)
	// sesh.SetMaxUploadRate(64)
	err = sesh.Start()
	if err != nil {
		panic(err)
	}
	start := time.Now()
	for start.Add(time.Second*60).After(time.Now()) && !sesh.Bundle.Complete {
		time.Sleep(time.Second)
	}
	defer sesh.Stop()
}

func TestPeer(tfPath string, myPeerID string) {
	fmt.Println("Starting peer test...")
	// Connect to a peer
	tf := torrentfile.New()
	err := tf.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	bdl, err := bundle.Create(tf, "./TestBundle")
	if err != nil {
		panic(err)
	}
	tl := trackerlist.New(tf.Announce, tf.AnnounceList, bdl.InfoHash, 6681, bdl.Length, false, myPeerID)
	err = tl.Start()
	if err != nil {
		fmt.Printf("Error starting tracker: %v\n", err)
		return
	}
	defer tl.Stop()
	var curPeer *peer.Peer
	peers := tl.GetPeers()
	for len(peers) == 0 {
		time.Sleep(50 * time.Millisecond)
		peers = tl.GetPeers()
	}
	for _, peerDetails := range peers {
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
			fmt.Printf("Current bitfield: ")
			bdl.Bitfield.Print()
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
