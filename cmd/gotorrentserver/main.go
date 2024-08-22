package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/api"
	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/client"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/trackerlist"
)

type Config struct {
	TorrentPort    int    `json:"torrent_port"`
	DBPath         string `json:"db_path"`
	MaxConnections int    `json:"max_connections"`
	APIPort        int    `json:"api_port"`
}

func main() {
	args := os.Args
	fmt.Println(args)

	if len(os.Args) > 2 {
		arg := os.Args[2]
		if arg == "test" {
			logger := log.Default()
			myPeerID := []byte("-TR2940-6oFw2M6BdUkY")
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
			default:
				panic(fmt.Errorf("pkg: %s tests not implemented", testPkg))
			}

		} else {
			fmt.Println("Server not implemented")
		}
	}

	// else run server

	cfg := Config{
		MaxConnections: 50,
		TorrentPort:    6681,
		DBPath:         "./gotorrentserver.db",
		APIPort:        8080,
	}
	cfgFile, err := os.OpenFile("./config.json", os.O_RDONLY, 0644)
	if err != nil {
		fmt.Printf("Error reading config: %v... Loaded default config", err)
	} else {
		defer cfgFile.Close()
		decoder := json.NewDecoder(cfgFile)
		err = decoder.Decode(&cfg)
		if err != nil {
			fmt.Printf("Error decoding config file: %v... Loaded default config", err)
		}
	}

	fmt.Println("Starting server")
	tc, err := client.Start(
		cfg.TorrentPort,
		cfg.DBPath,
		cfg.MaxConnections,
	)
	if err != nil {
		panic(err)
	}
	defer tc.Stop()
	handeRequests(tc, cfg.APIPort)

	fmt.Println("Stopped server")
}

func handeRequests(tc *client.TorrentClient, port int) {
	http.HandleFunc("/alldata", func(w http.ResponseWriter, r *http.Request) { api.AllData(w, r, tc) })
	http.HandleFunc("/torrentdata", func(w http.ResponseWriter, r *http.Request) { api.TorrentData(w, r, tc) })
	http.HandleFunc("/addtorrentfile", func(w http.ResponseWriter, r *http.Request) { api.AddTorrentFile(w, r, tc) })
	http.HandleFunc("/removetorrent", func(w http.ResponseWriter, r *http.Request) { api.RemoveTorrent(w, r, tc) })
	http.HandleFunc("/AddMagnet", func(w http.ResponseWriter, r *http.Request) { api.AddMagnet(w, r, tc) })
	http.HandleFunc("/stoptorrent", func(w http.ResponseWriter, r *http.Request) { api.StopTorrent(w, r, tc) })
	http.HandleFunc("/starttorrent", func(w http.ResponseWriter, r *http.Request) { api.StartTorrent(w, r, tc) })
	http.HandleFunc("/settorrentdownrate", func(w http.ResponseWriter, r *http.Request) { api.SetTorrentDownRate(w, r, tc) })
	http.HandleFunc("/settorrentuprate", func(w http.ResponseWriter, r *http.Request) { api.SetTorrentUpRate(w, r, tc) })
	http.HandleFunc("/setglobaldownrate", func(w http.ResponseWriter, r *http.Request) { api.SetGlobalDownRate(w, r, tc) })
	http.HandleFunc("/setglobaluprate", func(w http.ResponseWriter, r *http.Request) { api.SetGlobalUpRate(w, r, tc) })
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func TestTrackerList(tfPath string, myPeerID []byte, logger *log.Logger) {
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
	dbc, err := database.Connect("./gotorrentserver.db")
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

func TestSession(tfPath string, myPeerID []byte) {
	tf := torrentfile.New()
	err := tf.ParseFile(tfPath)
	if err != nil {
		panic(err)
	}
	fmt.Println("Torrentfile succesfully parsed")
	listenPort := uint16(6681)
	sesh, err := session.New("./TestBundle", tf, bitfield.New(int64(len(tf.Info.Pieces))), listenPort, myPeerID, 50)
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

func TestPeer(tfPath string, myPeerID []byte) {
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
