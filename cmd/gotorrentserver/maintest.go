package main

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

func main() {
	tf, err := torrentfile.ParseFile("./test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}

	port := 6681
	peerId := "-AZ2060-6wfG2wk6wWLc"

	t := tracker.New(tf.Announce, tf.InfoHash, port, 0, 0, tf.Info.Length, peerId)
	err = t.Start()
	if err != nil {
		panic(err)
	}
	fmt.Println("Tracker started...")
	fmt.Printf("TrackerID: %s\n", t.TrackerID)
	err = t.Stop()
	if err != nil {
		panic(err)
	}
}