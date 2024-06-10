package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/peer"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		panic(err)
	}

	tf, err := torrentfile.ParseFile("./test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}
	db.AddTorrentFile(string(tf.InfoHash), tf)

	port := 6681
	peerId := "IAZ2060V2wfG2wk6wWLc"

	t := tracker.New(tf.Announce, tf.InfoHash, port, 0, 0, tf.Info.Length, peerId)
	err = t.Start()
	if err != nil {
		panic(err)
	}
	fmt.Println("Tracker started...")
	fmt.Printf("TrackerID: %s\n", t.TrackerID)
	i := 0

	err = errors.New("connecting")
	var p *peer.Peer
	for err != nil && i < len(t.Peers) {
		peerDetails := t.Peers[i]
		fmt.Printf("Peer: %v\n", peerDetails)
		peerIP, ok := peerDetails["ip"].(string)
		if !ok {
			panic("peer IP not string")
		}
		peerPort, ok := peerDetails["port"].(int64)
		if !ok {
			panic("peer port not int")
		}
		fmt.Println(peerDetails)
		p, err = peer.New(peerIP, int(peerPort), port, string(tf.InfoHash), peerId)
		if err != nil {
			panic(err)
		}
		err = p.Connect()
		i++
	}
	if err != nil {
		panic(err)
	}
	time.Sleep(10 * time.Second)
	err = t.Stop()
	if err != nil {
		panic(err)
	}
}