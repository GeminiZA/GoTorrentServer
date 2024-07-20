package tracker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
	"unicode"

	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/bencode"
)

const DEBUG_TRACKER bool = true

type PeerInfo struct {
	PeerID string
	IP     string
	Port   int
}

type Tracker struct {
	instantiated bool
	TrackerUrl   string
	InfoHash     []byte
	Port         int
	TrackerID    string
	Downloaded   int64
	Uploaded     int64
	Left         int64
	PeerID       string
	Interval     time.Duration
	MinInterval  time.Duration
	Seeders      int64
	Leechers     int64
	Peers        []PeerInfo
	running      bool
	lastAnnounce time.Time
	mux          sync.Mutex
}

func New(trackerUrl string, infoHash []byte, port int, downloaded int64, uploaded int64, left int64, peerId string) *Tracker {
	tracker := Tracker{}
	tracker.InfoHash = infoHash
	tracker.TrackerUrl = trackerUrl
	tracker.Port = port
	tracker.Downloaded = downloaded
	tracker.Uploaded = uploaded
	tracker.Left = left
	tracker.PeerID = peerId
	tracker.TrackerID = ""
	tracker.Seeders = 0
	tracker.Leechers = 0
	tracker.instantiated = true
	tracker.running = false
	return &tracker
}

func (tracker *Tracker) parseBody(body []byte) error {
	bodyDict, err := bencode.Parse(&body)
	fmt.Printf("Successfully parsed body\n")
	if DEBUG_TRACKER {
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
			if DEBUG_TRACKER {
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
				if DEBUG_TRACKER {
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

	err := tracker.start()
	if err != nil {
		return err
	}

	tracker.running = true

	go tracker.run()

	return nil
}

func (tracker *Tracker) start() error {
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

	res, err := http.Get(fmt.Sprintf("%s?%s", tracker.TrackerUrl, params.Encode()))
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if DEBUG_TRACKER {
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
		if tracker.lastAnnounce.Add(tracker.MinInterval).After(time.Now()) {
			err := tracker.start()
			if err != nil {
				fmt.Printf("Error in tracker running: %v\n", err)
			} else {
				fmt.Printf("Tracker reannounced...\n")
			}
		}
	}
	fmt.Printf("Tracker stopped...\n")
}
