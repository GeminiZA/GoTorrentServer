package tracker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bencode"
)

type PeerInfo struct {
	PeerID string
	IP     string
	Port   int
}

type Tracker struct {
	instantiated       bool
	announcelist       [][]string
	announceErrorCount int
	TrackerUrl         string
	InfoHash           []byte
	Port               int
	TrackerID          string
	Downloaded         int64
	Uploaded           int64
	Left               int64
	PeerID             string
	Interval           time.Duration
	MinInterval        time.Duration
	Seeders            int64
	Leechers           int64
	Peers              []PeerInfo
	running            bool
	lastAnnounce       time.Time
	mux                sync.Mutex

	// UDP announce
	connectionID         uint32
	transactionID        uint32
	udpConn              *net.UDPConn
	udpAddr              *net.UDPAddr
	udpBuff              []byte
	connectionIDRecvTime time.Time
	timeoutStep          int
}

func New(announcelist [][]string, infoHash []byte, port int, downloaded int64, uploaded int64, left int64, peerId string) *Tracker {
	return &Tracker{
		InfoHash:           infoHash,
		announcelist:       announcelist,
		announceErrorCount: 0,
		TrackerUrl:         "",
		Port:               port,
		Downloaded:         downloaded,
		Uploaded:           uploaded,
		Left:               left,
		PeerID:             peerId,
		TrackerID:          "",
		Seeders:            0,
		Leechers:           0,
		instantiated:       true,
		running:            false,

		connectionID:         0,
		transactionID:        0,
		udpConn:              nil,
		udpBuff:              nil,
		connectionIDRecvTime: time.Time{},
		timeoutStep:          -1,
	}
}

func (tracker *Tracker) parseBody(body []byte) error {
	bodyDict, err := bencode.Parse(&body)
	fmt.Printf("Successfully parsed body\n")
	fmt.Printf("%v\n", bodyDict)
	if debugopts.TRACKER_DEBUG {
		// fmt.Printf("Tracker body response: %v\n", bodyDict)
	}
	if err != nil {
		return err
	}
	if failure, ok := bodyDict["failure reason"]; ok {
		return fmt.Errorf("Start failure; failure reasion: %v", failure)
	}
	if warning, ok := bodyDict["warning message"]; ok {
		fmt.Printf("Start Warning; warning message: %v", warning)
	}
	if interval, ok := bodyDict["interval"]; ok {
		if intervalStr, ok := interval.(string); ok {
			intervalInt, err := strconv.ParseInt(intervalStr, 10, 64)
			tracker.Interval = time.Duration(intervalInt) * time.Second
			if err != nil {
				return err
			}
		}
	} else {
		return errors.New("no interval in response")
	}
	if minInterval, ok := bodyDict["min interval"]; ok {
		if intervalStr, ok := minInterval.(string); ok {
			minIntervalInt, err := strconv.ParseInt(intervalStr, 10, 64)
			tracker.MinInterval = time.Duration(minIntervalInt) * time.Second
			if err != nil {
				return err
			}
		}
	} else {
		tracker.MinInterval = tracker.Interval - time.Second*5
	}
	if trackerID, ok := bodyDict["tracker id"]; ok {
		if trackerIDStr, ok := trackerID.(string); ok {
			tracker.TrackerID = trackerIDStr
		}
	}
	if complete, ok := bodyDict["complete"]; ok {
		if completeStr, ok := complete.(string); ok {
			tracker.Seeders, err = strconv.ParseInt(completeStr, 10, 64)
			if err != nil {
				return err
			}
		}
	}
	if incomplete, ok := bodyDict["incomplete"]; ok {
		if incompleteStr, ok := incomplete.(string); ok {
			tracker.Leechers, err = strconv.ParseInt(incompleteStr, 10, 64)
			if err != nil {
				return err
			}
		}
	}
	tracker.Peers = make([]PeerInfo, 0)
	if peers, ok := bodyDict["peers"]; ok {
		if peersListInter, ok := peers.([]interface{}); ok {
			peersList := make([]map[string]interface{}, 0)
			for _, inter := range peersListInter {
				if mp, ok := inter.(map[string]interface{}); ok {
					peersList = append(peersList, mp)
				}
			}
			if debugopts.TRACKER_DEBUG {
				fmt.Printf("Found peers: \n")
			}
			for _, peer := range peersList {
				peerID, ok := peer["peer id"].(string)
				if !ok {
					return errors.New("peer id not string")
				}
				skip := false
				for _, r := range peerID {
					if !unicode.IsPrint(r) {
						skip = true
						break
					}
				}
				if skip {
					fmt.Printf("Skipping unprintable peer ID: %x\n", peerID)
					continue
				}
				peerIP, ok := peer["ip"].(string)
				if !ok {
					return errors.New("peer ip not string")
				}
				peerPort, ok := peer["port"].(int64)
				if !ok {
					return errors.New("peerid not string")
				}
				if debugopts.TRACKER_DEBUG {
					fmt.Printf("PeerID: %x; IP: %s; Port: %d\n", peerID, peerIP, int(peerPort))
				}
				tracker.Peers = append(tracker.Peers, PeerInfo{PeerID: peerID, IP: peerIP, Port: int(peerPort)})
			}
		}
	} else {
		return errors.New("no peers in response")
	}
	return nil
}

func (tracker *Tracker) Start() error {
	if !tracker.instantiated {
		return errors.New("tracker incorrectly instantiated")
	}

	fmt.Printf("Infohash hex: %x\n", tracker.InfoHash)

	for _, list := range tracker.announcelist {
		for _, url := range list {
			tracker.TrackerUrl = url
			err := tracker.announce()
			if err != nil {
				fmt.Printf("error announcing to tracker (%s): %v\n", url, err)
			} else { // Successfully announced
				tracker.running = true
				go tracker.run()
				return nil
			}
		}
	}

	return errors.New("no more trackers to try")
}

func (tracker *Tracker) announce() error {
	tracker.mux.Lock()
	defer tracker.mux.Unlock()

	params := url.Values{}
	params.Add("info_hash", string(tracker.InfoHash))
	params.Add("peer_id", tracker.PeerID)
	params.Add("port", strconv.Itoa(tracker.Port))
	params.Add("uploaded", strconv.FormatInt(tracker.Uploaded, 10))
	params.Add("downloaded", strconv.FormatInt(tracker.Downloaded, 10))
	params.Add("left", strconv.FormatInt(tracker.Left, 10))
	params.Add("compact", "0")
	params.Add("event", "started")

	if strings.Contains(tracker.TrackerUrl, "udp://") {
		// udp announce
		// Make connection request
	} else {
		// http / https announce
		res, err := http.Get(fmt.Sprintf("%s?%s", tracker.TrackerUrl, params.Encode()))
		if err != nil {
			return err
		}
		if debugopts.TRACKER_DEBUG {
			fmt.Printf("Sending request to tracker: %s\n", fmt.Sprintf("%s?%s", tracker.TrackerUrl, params.Encode()))
		}

		defer res.Body.Close()

		if debugopts.TRACKER_DEBUG {
			fmt.Println("Got tracker response")
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		if body[0] != 'd' {
			return fmt.Errorf("tracker response error: %s", string(body))
		}

		err = tracker.parseBody(body)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tracker *Tracker) Complete() error {
	tracker.mux.Lock()
	defer tracker.mux.Unlock()

	if !tracker.running {
		return errors.New("tracker not running")
	}

	params := url.Values{}
	params.Add("info_hash", string(tracker.InfoHash))
	params.Add("peer_id", tracker.PeerID)
	params.Add("port", strconv.Itoa(tracker.Port))
	params.Add("uploaded", strconv.FormatInt(tracker.Uploaded, 10))
	params.Add("downloaded", strconv.FormatInt(tracker.Downloaded, 10))
	params.Add("left", strconv.FormatInt(tracker.Left, 10))
	params.Add("compact", "0")
	params.Add("event", "complete")

	res, err := http.Get(fmt.Sprintf("%s:%s", tracker.TrackerUrl, params.Encode()))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	err = tracker.parseBody(body)
	if err != nil {
		return err
	}

	return nil
}

func (tracker *Tracker) Stop() error {
	tracker.mux.Lock()
	defer tracker.mux.Unlock()

	if !tracker.running {
		return errors.New("tracker not running")
	}

	tracker.running = false

	params := url.Values{}
	params.Add("info_hash", string(tracker.InfoHash))
	params.Add("peer_id", tracker.PeerID)
	params.Add("port", strconv.Itoa(tracker.Port))
	params.Add("uploaded", strconv.FormatInt(tracker.Uploaded, 10))
	params.Add("downloaded", strconv.FormatInt(tracker.Downloaded, 10))
	params.Add("left", strconv.FormatInt(tracker.Left, 10))
	params.Add("compact", "0")
	params.Add("event", "stopped")

	res, err := http.Get(fmt.Sprintf("%s:%s", tracker.TrackerUrl, params.Encode()))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return nil
}

func (tracker *Tracker) run() {
	for tracker.running {
		time.Sleep(time.Second)
		if tracker.announceErrorCount > 0 || tracker.lastAnnounce.Add(tracker.MinInterval).After(time.Now()) {
			err := tracker.announce()
			if err != nil {
				tracker.announceErrorCount++
				fmt.Printf("Error in tracker running (%d/5): %v\n", tracker.announceErrorCount, err)
				if tracker.announceErrorCount > 5 { // Get new tracker
					fmt.Print("Getting new tracker...\n")
					found := false
					connected := false
					for _, list := range tracker.announcelist {
						if connected {
							break
						}
						for _, url := range list {
							if connected {
								break
							}
							if found {
								tracker.TrackerUrl = url
								err := tracker.announce()
								if err == nil {
									connected = true
								} else {
									fmt.Printf("error announcing to tracker(%s): %v\n", url, err)
								}
							} else {
								if url == tracker.TrackerUrl {
									found = true
								}
							}
						}
					}
				}
			} else {
				tracker.announceErrorCount = 0
				fmt.Printf("Tracker reannounced...\n")
			}
		}
	}
	fmt.Printf("Tracker stopped...\n")
}

// UDP announce

func (tracker *Tracker) udpAnnounce() error {
	if tracker.connectionIDRecvTime.IsZero() || time.Now().After(tracker.connectionIDRecvTime.Add(time.Minute)) {
		err := tracker.udpConnect()
	}
	return nil
}

func (tracker *Tracker) readUDP() error {
	if n == 0 {
		return errors.New("0 bytes read")
	}
	return nil
}

func (tracker *Tracker) udpConnect() error {
	addr, err := net.ResolveUDPAddr("udp", tracker.TrackerUrl)
	if err != nil {
		return err
	}
	tracker.udpAddr = addr
	conn, err := net.DialUDP("udp", nil, tracker.udpAddr)
	if err != nil {
		return err
	}
	tracker.udpConn = conn
	// {constant (8), action (4), transaction id (4)}
	connectRequestBytes := []byte{0x04, 0x17, 0x27, 0x10, 0x19, 0x80, 0x00, 0x00, 0x00, 0x00} // Protocol constant, 0 (connect)
	transIDBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(transIDBytes, uint32(rand.Int()))
	connectRequestBytes = append(connectRequestBytes, transIDBytes...)
	_, err = tracker.udpConn.Write(connectRequestBytes)
	if err != nil {
		return fmt.Errorf("5 timeouts: %v", err)
	}
	tracker.udpBuff = make([]byte, 1024)
	err = tracker.readUDP()
	for err != nil {
		if tracker.timeoutStep == 5 {
			return err
		}
		fmt.Printf("error reading udp response: %v\n", err)
		tracker.timeoutStep++
		time.Sleep(15 * (1 << tracker.timeoutStep) * time.Second)
		_, err = tracker.udpConn.Write(connectRequestBytes)
		if err != nil {
			return err
		}
		tracker.udpConn.SetReadDeadline(time.Now().Add(time.Second))
		var n int
		n, _, err = tracker.udpConn.ReadFromUDP(tracker.udpBuff)
		if n < 16 {
			err = errors.New("less than 16 bytes read")
		}
	}
	responseAction := binary.BigEndian.Uint32(tracker.udpBuff[0:4])
	responseTransactionID := binary.BigEndian.Uint32(tracker.udpBuff[4:8])
	responseConnectionID := binary.BigEndian.Uint32(tracker.udpBuff[8:16])
	return nil
}
