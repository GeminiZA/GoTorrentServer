package main

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/downloadfile"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

func main() {
	tf, err := torrentfile.ParseFile("test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Name: %s\nLength: %d\nPieceLength:%d\n", tf.Info.Name, tf.Info.Length, tf.Info.PieceLength)
	df, err := downloadfile.New(tf.Info.Name + ".download", tf.Info.Length, tf.Info.PieceLength, tf)
	if err != nil {
		panic(err)
	}
	fmt.Println(df.BitField)
}