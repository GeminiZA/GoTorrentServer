package main

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
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
}