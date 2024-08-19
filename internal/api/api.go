package api

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/client"
)

func AllData(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("alldata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	data, err := tc.AllDataJSON()
	if err != nil {
		fmt.Printf("Error getting all data: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func TorrentData(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("alldata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	infoHashHex := r.Header.Get("infoHashHex")
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error getting torrent data: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data, err := tc.TorrentDataJSON(infoHash)
	if err != nil {
		fmt.Printf("Error getting torrent data: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func RemoveTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("alldata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	deleteStr := r.Header.Get("delete")
	deleteFiles := deleteStr == "true"
	infoHashHex := r.Header.Get("infoHashHex")
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error removing torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = tc.RemoveTorrent(infoHash, deleteFiles)
	if err != nil {
		fmt.Printf("Error removing torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func AddMagnet(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func AddTorrentFile(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func PauseTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func ResumeTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func SetTorrentDownRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func SetTorrentUpRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func SetGlobalDownRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func SetGlobalUpRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}
