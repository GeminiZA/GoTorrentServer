package main

import (
	"fmt"
	"os"

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

				fmt.Println("Bundle created succesfully")

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
	for _, peerDetails := range trk.Peers {
		peer, err := peer.Connect(
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
		defer peer.Close()
	}
	fmt.Println("Peer test complete")
}
