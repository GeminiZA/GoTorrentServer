// TODO:
// Implemenent connections listenserver

package client

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/database"
	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bitfield"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/listenserver"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/session"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type TorrentClient struct {
	sessions     []*session.Session
	bitfields    []*bitfield.Bitfield
	dbc          *database.DBConn
	listenServer *listenserver.ListenServer // peerInChan   chan<- *peer.Peer
	peerInChan   chan *listenserver.IncomingPeer
	peerID       string
	running      bool
	mux          sync.Mutex

	torrentPort    int
	maxConnections int

	logger *logger.Logger
}

func Start(
	torrentPort int,
	dbPath string,
	maxConnections int,
) (*TorrentClient, error) {
	var err error
	client := &TorrentClient{
		sessions:       make([]*session.Session, 0),
		logger:         logger.New(logger.DEBUG, "TorrentClient"),
		peerInChan:     make(chan *listenserver.IncomingPeer, 10),
		maxConnections: maxConnections,
		torrentPort:    torrentPort,
	}
	client.dbc, err = database.Connect(dbPath)
	if err != nil {
		return nil, err
	}

	client.generatePeerID()

	infoHashes, err := client.dbc.GetAllInfoHashes()
	if err != nil {
		return nil, err
	}
	client.logger.Debug(fmt.Sprintf("Got infohashes: %v", infoHashes))
	for _, hash := range infoHashes {
		path, err := client.dbc.GetPath(hash)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to find path for infoHash: %x: %v", hash, err))
			continue
		}
		tfStr, err := client.dbc.GetTorrentFile(hash)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to get torrentfile for infoHash %x: %v", hash, err))
			continue
		}
		tf := torrentfile.New()
		tfData := []byte(tfStr)
		err = tf.ParseFileString(&tfData)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to parse torrentfile for infohash: %x: %v", hash, err))
			continue
		}
		bf, err := client.dbc.GetBitfield(hash)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to get bitfield for infohash: %x: %v", hash, err))
			continue
		}
		sesh, err := session.New(
			path,
			tf,
			bf,
			uint16(torrentPort),
			[]byte(client.peerID),
			client.maxConnections,
		)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to create session for infohash: %x: %v", hash, err))
			continue
		}
		status, err := client.dbc.GetStatus(hash)
		if err != nil {
			client.logger.Error(fmt.Sprintf("failed to create session for infohash: %x: %v", hash, err))
			continue
		}
		client.logger.Debug(fmt.Sprintf("Got status for torrent(%x): %d", hash, status))
		if status == 1 {
			client.logger.Debug(fmt.Sprintf("Starting session for torrent(%x)", hash))
			err = sesh.Start()
			if err != nil {
				client.logger.Error(fmt.Sprintf("failed to start session for infohash: %x: %v", hash, err))
				continue
			}
		}
		client.sessions = append(client.sessions, sesh)
		client.bitfields = append(client.bitfields, sesh.Bundle.Bitfield.Clone())
	}
	client.running = true
	go client.runClient()
	client.listenServer, err = listenserver.New(uint16(client.torrentPort), 0, "127.0.0.1", client.peerInChan)
	if err != nil {
		return client, err
	}
	err = client.listenServer.Start()
	if err != nil {
		return client, err
	}
	return client, nil
}

func (client *TorrentClient) Stop() error {
	client.mux.Lock()
	defer client.mux.Unlock()

	client.running = false
	for _, sesh := range client.sessions {
		sesh.Stop()
	}
	client.dbc.Disconnect()
	return nil
}

func (client *TorrentClient) AddTorrentFromFile(torrentfilePath string, targetPath string, start bool) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	tf := torrentfile.New()
	err := tf.ParseFile(torrentfilePath)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, tf.InfoHash) {
			return errors.New("torrent already exists in client")
		}
	}
	newSesh, err := session.New(targetPath, tf, bitfield.New(int64(len(tf.Info.Pieces))), uint16(client.torrentPort), []byte(client.peerID), client.maxConnections)
	if err != nil {
		return err
	}
	err = client.dbc.AddTorrent(tf, newSesh.Bundle.Bitfield, targetPath, newSesh.MaxDownloadRateKB, newSesh.MaxUploadRateKB, newSesh.Status)
	if err != nil {
		return err
	}
	client.sessions = append(client.sessions, newSesh)
	client.bitfields = append(client.bitfields, newSesh.Bundle.Bitfield.Clone())
	if start {
		err = client.StartTorrent(tf.InfoHash)
		if err != nil {
			return err
		}
	}
	return nil
}

func (client *TorrentClient) AddTorrentFromMetadata(metadata []byte, targetpath string, start bool) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	client.logger.Debug("Adding Torrent from metadata")
	tf := torrentfile.New()
	err := tf.ParseFileString(&metadata)
	if err != nil {
		return err
	}
	client.logger.Debug(fmt.Sprintf("Parsed metadata Successfully\nName: %s\nInfohash: %x", tf.Info.Name, tf.InfoHash))
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, tf.InfoHash) {
			return errors.New("session already exists")
		}
	}
	newSesh, err := session.New(targetpath, tf, bitfield.New(int64(len(tf.Info.Pieces))), uint16(client.torrentPort), []byte(client.peerID), client.maxConnections)
	if err != nil {
		return err
	}
	err = client.dbc.AddTorrent(tf, newSesh.Bundle.Bitfield, targetpath, newSesh.MaxDownloadRateKB, newSesh.MaxUploadRateKB, newSesh.Status)
	if err != nil {
		return err
	}
	client.sessions = append(client.sessions, newSesh)
	client.bitfields = append(client.bitfields, newSesh.Bundle.Bitfield.Clone())
	if start {
		err = client.StartTorrent(tf.InfoHash)
		if err != nil {
			return err
		}
	}
	return nil
}

func AddTorrentFromURL(magnet string) error {
	// Not implemented
	return errors.New("not yet implemented")
}

func (client *TorrentClient) StartTorrent(infoHash []byte) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.UpdateTorrentStatus(infoHash, 1)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.Start()
			return nil
		}
	}
	return errors.New("session doesn't exist")
}

func (client *TorrentClient) StopTorrent(infoHash []byte) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.UpdateTorrentStatus(infoHash, 0)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.Stop()
			return nil
		}
	}
	return errors.New("session for infoHash not found")
}

func (client *TorrentClient) RecheckTorrent(infoHash []byte) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.UpdateTorrentStatus(infoHash, 0)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.Recheck()
			return nil
		}
	}
	return errors.New("session for infoHash not found")
}

func (client *TorrentClient) SetTorrentDownloadRateKB(infoHash []byte, rateKB float64) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.UpdateDownloadRate(infoHash, rateKB)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.SetMaxDownloadRate(rateKB)
			return nil
		}
	}
	return errors.New("session for infoHash not found")
}

func (client *TorrentClient) SetTorrentUploadRateKB(infoHash []byte, rateKB float64) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.UpdateUploadRate(infoHash, rateKB)
	if err != nil {
		return err
	}
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.SetMaxUploadRate(rateKB)
			return nil
		}
	}
	return errors.New("session for infoHash not found")
}

func (client *TorrentClient) RemoveTorrent(infoHash []byte, delete bool) error {
	client.mux.Lock()
	defer client.mux.Unlock()

	if len(infoHash) != 20 {
		return errors.New("invalid InfoHash")
	}
	err := client.dbc.RemoveTorrent(infoHash)
	if err != nil {
		return err
	}
	seshIndex := -1
	for index, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infoHash) {
			sesh.Stop()
			if delete {
				err := sesh.Bundle.DeleteFiles()
				if err != nil {
					return err
				}
			}
			seshIndex = index
			break
		}
	}
	if seshIndex != -1 {
		client.sessions = append(client.sessions[:seshIndex], client.sessions[seshIndex+1:]...)
		client.bitfields = append(client.bitfields[:seshIndex], client.bitfields[seshIndex+1:]...)
		return err
	} else {
		return errors.New("session for infoHash not found")
	}
}

func (client *TorrentClient) runClient() {
	go client.processIncomingClients()
	for client.running {
		client.updateDatabase()
		time.Sleep(time.Millisecond * 1000)
		client.PrintStatus()
	}
}

func (client *TorrentClient) updateDatabase() {
	client.mux.Lock()
	defer client.mux.Unlock()

	for i := range client.sessions {
		if !client.sessions[i].Bundle.Bitfield.Equals(client.bitfields[i]) {
			err := client.dbc.UpdateBitfield(client.sessions[i].Bundle.InfoHash, client.sessions[i].Bundle.Bitfield)
			if err != nil {
				client.logger.Error(fmt.Sprintf("Error updating database bitfield %v", err))
			} else {
				client.bitfields[i] = client.sessions[i].Bundle.Bitfield.Clone()
			}
		}
	}
}

func (client *TorrentClient) processIncomingClients() {
	for client.running {
		newPeer := <-client.listenServer.PeerChan
		found := false
		for _, session := range client.sessions {
			if bytes.Equal(session.Bundle.InfoHash, newPeer.InfoHash) {
				session.AddPeer(newPeer)
				found = true
				break
			}
		}
		if !found {
			client.logger.Error(fmt.Sprintf("Error on Peer(%s) trying to connect: Not serving infohash: %x", newPeer.Conn.RemoteAddr().String(), newPeer.InfoHash))
		}
	}
}

// Info Structs

type TrackerInfo struct {
	Url          string `json:"url"`
	Status       string `json:"status"`
	Leechers     int    `json:"leechers"`
	Seeders      int    `json:"seeders"`
	Peers        int    `json:"peers"`
	LastAnnounce string `json:"lastannounce"`
}

type PeerInfo struct {
	ID             string  `json:"peerid"`
	Host           string  `json:"host"`
	BitfieldB64    string  `json:"bitfield_bajk64"`
	BitfieldLength int     `json:"bitfield_length"`
	DownRate       float64 `json:"downrate"`
	UpRate         float64 `json:"uprate"`
	Downloaded     float64 `json:"downloaded"`
	Uploaded       float64 `json:"uploaded"`
	Connected      bool    `json:"connected"`
	Relevance      float64 `json:"relevance"`
}

type SessionInfo struct {
	Name           string        `json:"name"`
	InfoHashB64    string        `json:"infohash_base64"`
	BitfieldB64    string        `json:"bitfield_base64"`
	BitFieldLength int           `json:"bitfield_length"`
	TimeStarted    string        `json:"time"`
	Wasted         float64       `json:"wasted"`
	Downloaded     float64       `json:"downloaded"`
	Uploaded       float64       `json:"uploaded"`
	Trackers       []TrackerInfo `json:"trackers"`
	Peers          []PeerInfo    `json:"peers"`
	Error          string        `json:"error"`
}

type AllInfo struct {
	Time     string        `json:"time"`
	Sessions []SessionInfo `json:"sessions"`
}

func (client *TorrentClient) AllDataJSON() ([]byte, error) {
	ret := AllInfo{
		Time:     time.Now().String(),
		Sessions: make([]SessionInfo, 0),
	}
	for _, sesh := range client.sessions {
		newSeshInfo := SessionInfo{
			Name:           sesh.Bundle.Name,
			InfoHashB64:    base64.StdEncoding.EncodeToString(sesh.Bundle.InfoHash),
			BitfieldB64:    base64.StdEncoding.EncodeToString(sesh.Bundle.Bitfield.Bytes),
			BitFieldLength: int(sesh.Bundle.Bitfield.Len()),
			Wasted:         0,
			Downloaded:     sesh.TotalDownloadedKB,
			Uploaded:       sesh.TotalUploadedKB,
			Trackers:       make([]TrackerInfo, 0),
			Peers:          make([]PeerInfo, 0),
			Error:          fmt.Sprintf("%v", sesh.Error),
		}
		for _, peer := range sesh.Peers {
			newSeshInfo.Peers = append(newSeshInfo.Peers, PeerInfo{
				ID:             string(peer.RemotePeerID),
				Host:           fmt.Sprintf("%s:%d", peer.RemoteIP, peer.RemotePort),
				BitfieldB64:    base64.StdEncoding.EncodeToString(peer.RemoteBitfield.Bytes),
				BitfieldLength: int(peer.RemoteBitfield.Len()),
				DownRate:       peer.DownloadRateKB,
				UpRate:         peer.UploadRateKB,
				Downloaded:     float64(peer.TotalBytesDownloaded) / 1024,
				Uploaded:       float64(peer.TotalBytesUploaded) / 1024,
				Connected:      peer.Connected,
				Relevance:      peer.GetRelevance(sesh.Bundle.Bitfield),
			})
		}
		for _, tr := range sesh.TrackerList.Trackers {
			trerr := tr.TrackerError
			if trerr == nil {
				trerr = errors.New("null")
			}
			newSeshInfo.Trackers = append(newSeshInfo.Trackers, TrackerInfo{
				Url:          tr.TrackerUrl,
				Seeders:      int(tr.Seeders),
				Leechers:     int(tr.Leechers),
				Status:       trerr.Error(),
				Peers:        len(tr.Peers),
				LastAnnounce: tr.LastAnnounce.String(),
			})
		}
		ret.Sessions = append(ret.Sessions, newSeshInfo)
	}
	return json.Marshal(ret)
}

func (client *TorrentClient) TorrentDataJSON(infohash []byte) ([]byte, error) {
	for _, sesh := range client.sessions {
		if bytes.Equal(sesh.Bundle.InfoHash, infohash) {
			ret := SessionInfo{
				Name:           sesh.Bundle.Name,
				InfoHashB64:    base64.StdEncoding.EncodeToString(sesh.Bundle.InfoHash),
				BitfieldB64:    base64.StdEncoding.EncodeToString(sesh.Bundle.Bitfield.Bytes),
				BitFieldLength: int(sesh.Bundle.Bitfield.Len()),
				Wasted:         0,
				Downloaded:     sesh.TotalDownloadedKB,
				Uploaded:       sesh.TotalUploadedKB,
				Trackers:       make([]TrackerInfo, 0),
				Error:          fmt.Sprintf("%v", sesh.Error),
			}
			for _, peer := range sesh.Peers {
				ret.Peers = append(ret.Peers, PeerInfo{
					ID:             string(peer.RemotePeerID),
					Host:           fmt.Sprintf("%s:%d", peer.RemoteIP, peer.RemotePort),
					BitfieldB64:    base64.StdEncoding.EncodeToString(peer.RemoteBitfield.Bytes),
					BitfieldLength: int(peer.RemoteBitfield.Len()),
					DownRate:       peer.DownloadRateKB,
					UpRate:         peer.UploadRateKB,
					Downloaded:     float64(peer.TotalBytesDownloaded) / 1024,
					Uploaded:       float64(peer.TotalBytesUploaded) / 1024,
					Connected:      peer.Connected,
					Relevance:      peer.GetRelevance(sesh.Bundle.Bitfield),
				})
			}
			for _, tr := range sesh.TrackerList.Trackers {
				trerr := tr.TrackerError
				if trerr == nil {
					trerr = errors.New("null")
				}
				ret.Trackers = append(ret.Trackers, TrackerInfo{
					Url:          tr.TrackerUrl,
					Seeders:      int(tr.Seeders),
					Leechers:     int(tr.Leechers),
					Status:       trerr.Error(),
					Peers:        len(tr.Peers),
					LastAnnounce: tr.LastAnnounce.String(),
				})
			}
			return json.Marshal(ret)
		}
	}
	return nil, errors.New("infohash not found in sessions")
}

func (client *TorrentClient) generatePeerID() {
	name := "GTS-"
	time := time.Now()
	temp := []byte(name + time.String())
	hasher := sha1.New()
	hasher.Write(temp)
	hash := hasher.Sum(nil)
	client.peerID = fmt.Sprintf("GTS-%s", base64.StdEncoding.EncodeToString(hash)[0:16])
}

func (client *TorrentClient) PrintStatus() {
	ret := "Infohash\t\t\t\t\tConnected\tConnecting\tUnconnected\tDownloaded            Uploaded              Download Rate         Upload Rate           Size                  Progress\n"
	for _, sesh := range client.sessions {
		downloadedKiB := sesh.TotalDownloadedKB
		var downloadedStr string
		if downloadedKiB < 10240 {
			downloadedStr = fmt.Sprintf("%.2f KiB", sesh.TotalDownloadedKB)
			downloadedStr = fmt.Sprintf("%s%*s", downloadedStr, 22-len(downloadedStr), " ")
		} else if downloadedKiB < 1024*10240 {
			downloadedStr = fmt.Sprintf("%.2f MiB", sesh.TotalDownloadedKB/1024)
			downloadedStr = fmt.Sprintf("%s%*s", downloadedStr, 22-len(downloadedStr), " ")
		} else {
			downloadedStr = fmt.Sprintf("%.2f GiB", sesh.TotalDownloadedKB/1024/1024)
			downloadedStr = fmt.Sprintf("%s%*s", downloadedStr, 22-len(downloadedStr), " ")
		}
		var uploadedStr string
		uploadedKiB := sesh.TotalUploadedKB
		if uploadedKiB < 10240 {
			uploadedStr = fmt.Sprintf("%.2f KiB", sesh.TotalUploadedKB)
			uploadedStr = fmt.Sprintf("%s%*s", uploadedStr, 22-len(uploadedStr), " ")
		} else if uploadedKiB < 1024*10240 {
			uploadedStr = fmt.Sprintf("%.2f MiB", sesh.TotalUploadedKB/1024)
			uploadedStr = fmt.Sprintf("%s%*s", uploadedStr, 22-len(uploadedStr), " ")
		} else {
			uploadedStr = fmt.Sprintf("%.2f GiB", sesh.TotalUploadedKB/1024/1024)
			uploadedStr = fmt.Sprintf("%s%*s", uploadedStr, 22-len(uploadedStr), " ")
		}
		var downrateStr string
		downrate := sesh.DownloadRateKB
		if downrate < 1024 {
			downrateStr = fmt.Sprintf("%.2f KiB/s", downrate)
			downrateStr = fmt.Sprintf("%s%*s", downrateStr, 22-len(downrateStr), " ")
		} else {
			downrateStr = fmt.Sprintf("%.2f MiB/s", downrate/1024)
			downrateStr = fmt.Sprintf("%s%*s", downrateStr, 22-len(downrateStr), " ")
		}
		var uprateStr string
		uprate := sesh.UploadRateKB
		if uprate < 1024 {
			uprateStr = fmt.Sprintf("%.2f KiB/s", uprate)
			uprateStr = fmt.Sprintf("%s%*s", uprateStr, 22-len(uprateStr), " ")
		} else {
			uprateStr = fmt.Sprintf("%.2f MiB/s", uprate/1024)
			uprateStr = fmt.Sprintf("%s%*s", uprateStr, 22-len(uprateStr), " ")
		}
		totalSizeStr := fmt.Sprintf("%.2f MiB", float64(sesh.Bundle.Length)/1024/1024)
		totalSizeStr = fmt.Sprintf("%s%*s", totalSizeStr, 22-len(totalSizeStr), " ")
		ret += fmt.Sprintf("%x\t\t%d\t\t%d\t\t%d\t\t%s%s%s%s%s%.2f%%\n", sesh.Bundle.InfoHash, len(sesh.ConnectedPeers), len(sesh.ConnectingPeers), len(sesh.UnconnectedPeers), downloadedStr, uploadedStr, downrateStr, uprateStr, totalSizeStr, float64(sesh.Bundle.Bitfield.NumSet*100)/float64(sesh.Bundle.Bitfield.Len()))
	}
	ret += "==================================================\n"
	fmt.Print(ret)
}
