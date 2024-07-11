package main

import (
	"fmt"
	"os"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bundle"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
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
				tf, err := torrentfile.ParseFile(os.Args[4])
				if err != nil {
					panic(err)
				}
				fmt.Println("Torrentfile succesfully parsed")
				_, err = bundle.NewBundle(tf, "./TestBundle", 20)
				if err != nil {
					panic(err)
				}
			} else if testPkg == "session" {
				// Session tests
			} else {
				panic(fmt.Errorf("pkg: %s tests not implemented", testPkg))
			}
		}
	}
}

