package main

import (
	"fmt"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
)

func main() {
	db, err := database.Connect()
	defer db.Disconnect()
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	l, err := db.GetAllFiles()

	if err != nil {
		panic(err)
	}

	for i, item := range l {
		fmt.Printf("Index: %v Item: %v", i, item)
	}

	//tf := database.TargetFile {
		//InfoHash: "asdf1123",
		//Path: "./testpath.tf",
		//Time: time.Now().String(),
		//Description: "A Test TargetFile",
		//TrackerURL: "http://example.com",
	//}

	//if err := db.AddFile(tf); err != nil {
		//panic(err)
	//}

	
	//if err := db.AddFile(tf); err != nil {
		//panic(err)
	//}



}