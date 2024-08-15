package api

import (
	"fmt"
	"net/http"
)

func AllData(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "All Data Endpoint hit")
}

func TorrentData(w http.ResponseWriter, r *http.Request) {
}

func RemoveTorrent(w http.ResponseWriter, r *http.Request) {
}

func AddMagnet(w http.ResponseWriter, r *http.Request) {
}

func AddTorrentFile(w http.ResponseWriter, r *http.Request) {
}

func PauseTorrent(w http.ResponseWriter, r *http.Request) {
}

func ResumeTorrent(w http.ResponseWriter, r *http.Request) {
}

func SetTorrentDownRate(w http.ResponseWriter, r *http.Request) {
}

func SetTorrentUpRate(w http.ResponseWriter, r *http.Request) {
}

func SetGlobalDownRate(w http.ResponseWriter, r *http.Request) {
}

func SetGlobalUpRate(w http.ResponseWriter, r *http.Request) {
}
