package main

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
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
	bundle, err := bundle.Create(fmt.Sprintf("./%s", tf.Info.Name), tf)
	if err != nil {
		panic(err)
	}
	fmt.Println("Bundle created")
	for _, file := range bundle.Files {
		fmt.Printf("File: %s\n", file.Path)
	}
	fmt.Printf("Pieces: %v\n", bundle.Pieces)
	//for i, piece := range bundle.Pieces {
		//fmt.Printf("Piece number: %d / %d\n", i, len(bundle.Pieces))
		//for _, block := range piece.Blocks {
			//fmt.Printf("Block: offset: %d, length: %d\n", block.ByteOffset, block.Length)
		//}
	//}
	bundle.BitField.Print()
	err = bundle.WriteBlock(0, 0, []byte{0xFF, 0xFF})
	if err != nil {
		panic(err)
	}
}