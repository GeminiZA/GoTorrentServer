package api

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/client"
)

func AllData(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("alldata", remoteHost, http.StatusMethodNotAllowed)
		return
	}
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
		logResponse("alldata", remoteHost, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
	logResponse("alldata", remoteHost, http.StatusOK)
}

func TorrentData(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("torrentdata", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("torrentdata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("torrentdata", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error getting torrent data: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("torrentdata", remoteHost, http.StatusInternalServerError)
		return
	}
	data, err := tc.TorrentDataJSON(infoHash)
	if err != nil {
		fmt.Printf("Error getting torrent data: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("torrentdata", remoteHost, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
	logResponse("torrentdata", remoteHost, http.StatusOK)
}

func RemoveTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("removetorrent", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("removetorrent", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	deleteStr := values.Get("delete")
	deleteFiles := deleteStr == "true"
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("removetorrent", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error removing torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("removetorrent", remoteHost, http.StatusInternalServerError)
		return
	}
	err = tc.RemoveTorrent(infoHash, deleteFiles)
	if err != nil {
		fmt.Printf("Error removing torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	logResponse("removetorrent", remoteHost, http.StatusOK)
}

func AddMagnet(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
	logResponse("addmagnet", remoteHost, http.StatusNotImplemented)
}

func AddTorrentFile(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("alldata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
}

func StopTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("stoptorrent", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("stoptorrent", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("stoptorrent", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error stopping torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("stoptorrent", remoteHost, http.StatusInternalServerError)
		return
	}
	err = tc.StopTorrent(infoHash)
	if err != nil {
		fmt.Printf("Error stopping torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("stoptorrent", remoteHost, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	logResponse("stoptorrent", remoteHost, http.StatusOK)
}

func StartTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("alldata", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("alldata", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("starttorrent", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error starting torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("starttorrent", remoteHost, http.StatusInternalServerError)
		return
	}
	err = tc.StartTorrent(infoHash)
	if err != nil {
		fmt.Printf("Error removing torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("starttorrent", remoteHost, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	logResponse("starttorrent", remoteHost, http.StatusOK)
}

func SetTorrentDownRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("settorrentdownrate", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("settorrentdownrate", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("settorrentdownrate", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error setting rate: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
		logResponse("settorrentdownrate", remoteHost, http.StatusUnprocessableEntity)
		return
	}
	rate := values.Get("rate")
	rateKB, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		fmt.Printf("Error setting rate: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
		logResponse("settorrentdownrate", remoteHost, http.StatusUnprocessableEntity)
		return
	}
	err = tc.SetTorrentDownloadRateKB(infoHash, rateKB)
	if err != nil {
		fmt.Printf("Error settinging download rate on torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("settorrentdownrate", remoteHost, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func SetTorrentUpRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		logResponse("settorrentuprate", remoteHost, http.StatusMethodNotAllowed)
		return
	}
	if !isPrivateIP(net.IP(remoteHost)) {
		err := Authenticate(DevAlwaysTrue)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			logResponse("settorrentuprate", remoteHost, http.StatusUnauthorized)
			return
		}
	}
	values := r.URL.Query()
	infoHashHex := values.Get("infohash")
	if len(infoHashHex) != 40 {
		http.Error(w, "Infohash missing", http.StatusUnprocessableEntity)
		logResponse("settorrentuprate", remoteHost, http.StatusUnprocessableEntity)
	}
	infoHash, err := hex.DecodeString(infoHashHex)
	if err != nil {
		fmt.Printf("Error setting rate: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
		logResponse("settorrentuprate", remoteHost, http.StatusUnprocessableEntity)
		return
	}
	rate := values.Get("rate")
	rateKB, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		fmt.Printf("Error setting rate: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusUnprocessableEntity)
		logResponse("settorrentuprate", remoteHost, http.StatusUnprocessableEntity)
		return
	}
	err = tc.SetTorrentUploadRateKB(infoHash, rateKB)
	if err != nil {
		fmt.Printf("Error setting upload rate on torrent: %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logResponse("settorrentuprate", remoteHost, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func SetGlobalDownRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
	logResponse("setglobaldownrate", remoteHost, http.StatusNotImplemented)
}

func SetGlobalUpRate(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
	remoteHost := r.RemoteAddr
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
	logResponse("setglobaluprate", remoteHost, http.StatusNotImplemented)
}
