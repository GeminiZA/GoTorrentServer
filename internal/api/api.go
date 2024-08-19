package api

import (
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
}

func TorrentData(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
}

func RemoveTorrent(w http.ResponseWriter, r *http.Request, tc *client.TorrentClient) {
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
