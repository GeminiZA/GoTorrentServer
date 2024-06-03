package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/GeminiZA/GoTorrentServer/internal/api"
	"github.com/gorilla/mux"
)

func handleRequests() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/getInfo", api.GetInfo)
	log.Fatal(http.ListenAndServe(":10000", router))
}

func main() {
	fmt.Println("Starting server...")
	handleRequests()
}