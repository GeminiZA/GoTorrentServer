// TODO:
// Implemenent connections listenserver

package client

import (
	"bytes"
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

const (
	ListenPort uint16 = 6881
	PeerIDStr  string = "-TR2940-6oFw2M6BdUkY"
)

type TorrentClient struct {
	sessions     []*session.Session
	bitfields    []*bitfield.Bitfield
	dbc          *database.DBConn
	listenServer *listenserver.ListenServer
	// peerInChan   chan<- *peer.Peer
	running bool
	mux     sync.Mutex

	logger *logger.Logger
}

func Start() (*TorrentClient, error) {
	var err error
	client := &TorrentClient{
		sessions: make([]*session.Session, 0),
		logger:   logger.New(logger.DEBUG, "TorrentClient"),
		// peerInChan: make(chan<- *peer.Peer, 10),
	}
	client.dbc, err = database.Connect()
	if err != nil {
		return nil, err
	}

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
			ListenPort,
			[]byte(PeerIDStr),
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
	}
	client.running = true
	// client.listenServer, err = listenserver.New(ListenPort, client.peerInChan)
	//	if err != nil {
	// 	return client, err
	// }
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
	err = client.dbc.AddTorrent(tf, bitfield.New(int64(len(tf.Info.Pieces))), targetPath, 0, 0, 1)
	if err != nil {
		return err
	}
	newSesh, err := session.New(targetPath, tf, bitfield.New(int64(len(tf.Info.Pieces))), ListenPort, []byte(PeerIDStr))
	if err != nil {
		return err
	}
	client.sessions = append(client.sessions, newSesh)
	client.bitfields = append(client.bitfields, newSesh.Bundle.Bitfield.Clone())
	if start {
		err = newSesh.Start()
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
		return nil
	} else {
		return errors.New("session for infoHash not found")
	}
}

func (client *TorrentClient) runClient() {
	go client.processIncomingClients()
	for client.running {
		client.updateDatabase()
		time.Sleep(time.Millisecond * 1000)
	}
}

func (client *TorrentClient) updateDatabase() {
	client.mux.Lock()
	defer client.mux.Unlock()

	for i := range client.sessions {
		if !client.sessions[i].Bundle.Bitfield.Equals(client.bitfields[i]) {
			client.dbc.UpdateBitfield(client.sessions[i].Bundle.InfoHash, client.sessions[i].Bundle.Bitfield)
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
	ID          string  `json:"peerid"`
	Host        string  `json:"host"`
	BitfieldHex string  `json:"bitfieldhex"`
	Down        float64 `json:"down"`
	Up          float64 `json:"up"`
	Status      string  `json:"status"`
}

type SessionInfo struct {
	Name           string        `json:"name"`
	InfoHashB64    string        `json:"infohash_base64"`
	BitfieldB64    string        `json:"bitfield_base64"`
	BitFieldLength int           `json:"bitfield_length`
	TimeStarted    string        `json:"time"`
	Wasted         float64       `json:"wasted"`
	Downloaded     float64       `json:"downloaded"`
	Uploaded       float64       `json:"uploaded"`
	Trackers       []TrackerInfo `json:"trackers"`
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
			Error:          fmt.Sprintf("%v", sesh.Error),
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
