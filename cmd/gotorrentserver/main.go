package main

import (
	"github.com/GeminiZA/GoTorrentServer/internal/torrentfile"
)

func main() {
	_, err := torrentfile.ParseFile("test_folder-d984f67af9917b214cd8b6048ab5624c7df6a07a.torrent")
	if err != nil {
		panic(err)
	}
}