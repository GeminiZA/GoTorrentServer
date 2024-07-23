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

	"github.com/GeminiZA/GoTorrentServer/internal/debugopts"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bencode"
)

type PeerInfo struct {
	IP   string
	Port int
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
	complete           bool
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
	connectionID         uint64
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
		complete:           false,
		PeerID:             peerId,
		TrackerID:          "",
		Seeders:            0,
		Leechers:           0,
		instantiated:       true,
		running:            false,

		connectionID:         0,
		udpConn:              nil,
		udpBuff:              nil,
		connectionIDRecvTime: time.Time{},
		timeoutStep:          0,
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
				peerIP, ok := peer["ip"].(string)
				if !ok {
					return errors.New("peer ip not string")
				}
				peerPort, ok := peer["port"].(int64)
				if !ok {
					return errors.New("peerid not string")
				}
				if debugopts.TRACKER_DEBUG {
					fmt.Printf("IP: %s; Port: %d\n", peerIP, int(peerPort))
				}
				tracker.Peers = append(tracker.Peers, PeerInfo{IP: peerIP, Port: int(peerPort)})
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

	for _, list := range tracker.announcelist {
		for _, url := range list {
			tracker.TrackerUrl = url
			err := tracker.announce(2)
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

func (tracker *Tracker) announce(event int) error {
	tracker.mux.Lock()
	defer tracker.mux.Unlock()

	if strings.Contains(tracker.TrackerUrl, "udp://") {
		err := tracker.udpAnnounce(event)
		if err != nil {
			return err
		}
		return nil
	}

	params := url.Values{}
	params.Add("info_hash", string(tracker.InfoHash))
	params.Add("peer_id", tracker.PeerID)
	params.Add("port", strconv.Itoa(tracker.Port))
	params.Add("uploaded", strconv.FormatInt(tracker.Uploaded, 10))
	params.Add("downloaded", strconv.FormatInt(tracker.Downloaded, 10))
	params.Add("left", strconv.FormatInt(tracker.Left, 10))
	params.Add("compact", "0")
	switch event {
	case 0:
		params.Add("event", "none")
	case 1:
		params.Add("event", "completed")
	case 2:
		params.Add("event", "started")
	case 3:
		params.Add("event", "stopped")
	}

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
	return nil
}

func (tracker *Tracker) Complete() error {
	tracker.mux.Lock()
	defer tracker.mux.Unlock()

	tracker.complete = true

	if !tracker.running {
		return errors.New("tracker not running")
	}
	err := tracker.announce(1)
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

	err := tracker.announce(3)
	if err != nil {
		return err
	}

	return nil
}

func (tracker *Tracker) run() {
	for tracker.running {
		time.Sleep(time.Second)
		if tracker.announceErrorCount > 0 || tracker.lastAnnounce.Add(tracker.MinInterval).After(time.Now()) {
			err := tracker.announce(0)
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
								err := tracker.announce(0)
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

func (tracker *Tracker) udpAnnounce(event int) error { // event 0: none, 1: completed, 2: started, 3: stopped
	if tracker.connectionIDRecvTime.IsZero() ||
		time.Now().After(tracker.connectionIDRecvTime.Add(time.Minute)) ||
		tracker.udpAddr == nil ||
		tracker.udpConn == nil {
		err := tracker.udpConnect()
		if err != nil {
			return err
		}
	}
	transactionID := uint32(rand.Int())
	announceBytes := make([]byte, 98)
	binary.BigEndian.PutUint64(announceBytes[0:], tracker.connectionID) // connectionID
	binary.BigEndian.PutUint32(announceBytes[8:], 1)                    // Action 1 => announce
	binary.BigEndian.PutUint32(announceBytes[12:], transactionID)
	copy(announceBytes[16:36], tracker.InfoHash)
	copy(announceBytes[36:56], []byte(tracker.PeerID))
	binary.BigEndian.PutUint64(announceBytes[56:], uint64(tracker.Downloaded))
	binary.BigEndian.PutUint64(announceBytes[64:], uint64(tracker.Left))
	binary.BigEndian.PutUint64(announceBytes[72:], uint64(tracker.Uploaded))
	binary.BigEndian.PutUint32(announceBytes[80:], uint32(event))      // event 0: none; 1: completed; 2: started; 3: stopped;
	binary.BigEndian.PutUint32(announceBytes[84:], 0)                  // ip address // default: 0
	binary.BigEndian.PutUint32(announceBytes[88:], uint32(rand.Int())) // key // used for ipv6
	binary.BigEndian.PutUint32(announceBytes[92:], uint32(0xFFFFFFFF)) // num wanted // defaulted to -1
	binary.BigEndian.PutUint16(announceBytes[96:], uint16(tracker.Port))
	_, err := tracker.udpConn.Write(announceBytes)
	if err != nil {
		return err
	}
	if event == 3 { // if announcing stopped do not wait for a reply
		return nil
	}
	responseBuffer := make([]byte, 128)
	responseLength := 0
	for tracker.timeoutStep < 5 {
		if time.Now().After(tracker.connectionIDRecvTime.Add(time.Minute)) {
			return errors.New("connection id expired")
		}
		tracker.udpConn.SetReadDeadline(time.Now().Add((15 * (1 << tracker.timeoutStep)) * time.Second))
		n, _, err := tracker.udpConn.ReadFromUDP(responseBuffer)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				fmt.Printf("err reading udp tracker announce response: %v\n", err)
				tracker.timeoutStep++
				continue
			} else {
				return err
			}
		}
		if n < 20 {
			return errors.New("udp tracker announce response < 20 bytes")
		} else {
			responseLength = n
			tracker.timeoutStep = 0
			break
		}
	}
	if tracker.timeoutStep == 5 {
		return errors.New("tracker announce timed out")
	}
	responseAction := binary.BigEndian.Uint32(responseBuffer[0:4])
	if responseAction != 1 {
		return errors.New("udp tracker announce response not action: announce")
	}
	responseTransactionID := binary.BigEndian.Uint32(responseBuffer[4:8])
	if responseTransactionID != transactionID {
		return errors.New("udp tracker announce transaction ID mismatch")
	}
	responseInterval := binary.BigEndian.Uint32(responseBuffer[8:12])
	tracker.MinInterval = (time.Second * time.Duration(responseInterval))
	responseLeechers := binary.BigEndian.Uint32(responseBuffer[12:16])
	tracker.Leechers = int64(responseLeechers)
	responseSeeders := binary.BigEndian.Uint32(responseBuffer[16:20])
	tracker.Seeders = int64(responseSeeders)
	newPeers := make([]PeerInfo, 0)
	for curByteIndex := 20; curByteIndex+6 < responseLength; curByteIndex += 6 {
		newPeerIPBytes := responseBuffer[curByteIndex : curByteIndex+4]
		newPeerIPStr := fmt.Sprintf("%d:%d:%d:%d", newPeerIPBytes[0], newPeerIPBytes[1], newPeerIPBytes[2], newPeerIPBytes[3])
		newPeerPort := int(binary.BigEndian.Uint16(responseBuffer[curByteIndex+4 : curByteIndex+6]))
		for _, peer := range tracker.Peers {
			if peer.IP == newPeerIPStr && peer.Port == newPeerPort {
				// already have peer
				continue
			}
			newPeer := PeerInfo{
				IP:   newPeerIPStr,
				Port: newPeerPort,
			}
			newPeers = append(newPeers, newPeer)
		}
	}
	tracker.Peers = append(tracker.Peers, newPeers...)
	return nil
}

func (tracker *Tracker) udpConnect() error {
	url, err := url.Parse(tracker.TrackerUrl)
	if err != nil {
		return err
	}
	addr, err := net.ResolveUDPAddr("udp", url.Host)
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
	transactionID := uint32(rand.Int())
	binary.BigEndian.PutUint32(transIDBytes, transactionID)
	connectRequestBytes = append(connectRequestBytes, transIDBytes...)
	_, err = tracker.udpConn.Write(connectRequestBytes)
	if err != nil {
		return err
	}
	responseBuffer := make([]byte, 128)
	for tracker.timeoutStep < 5 {
		tracker.udpConn.SetReadDeadline(time.Now().Add((15 * (1 << tracker.timeoutStep)) * time.Second))
		n, _, err := tracker.udpConn.ReadFromUDP(responseBuffer)
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				fmt.Printf("err (%d/5) reading udp tracker connect response: %v\n", tracker.timeoutStep, err)
				tracker.timeoutStep++
				continue
			} else {
				return err
			}
		}
		if n < 16 {
			return errors.New("udp tracker response < 16 bytes")
		} else {
			tracker.timeoutStep = 0
			break
		}
	}
	if tracker.timeoutStep == 5 {
		return errors.New("tracker connect timed out 5 times")
	}
	responseAction := binary.BigEndian.Uint32(responseBuffer[0:4])
	responseTransactionID := binary.BigEndian.Uint32(responseBuffer[4:8])
	responseConnectionID := binary.BigEndian.Uint64(responseBuffer[8:16])

	if responseTransactionID != transactionID {
		return errors.New("udp tracker connect transaction ID mismatch")
	}
	if responseAction != 0 { // 0 => connect
		return errors.New("udp tracker connect action not connect")
	}
	tracker.connectionIDRecvTime = time.Now()
	tracker.connectionID = responseConnectionID
	return nil
}
