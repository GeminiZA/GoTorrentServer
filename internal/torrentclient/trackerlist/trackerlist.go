package trackerlist

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GeminiZA/GoTorrentServer/internal/logger"
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/tracker"
)

type TrackerList struct {
	announceList [][]string
	Trackers     []*tracker.Tracker
	infoHash     []byte
	port         uint16
	downloaded   int64
	uploaded     int64
	left         int64
	complete     bool
	peerID       []byte
	Seeders      int
	Leechers     int
	Peers        []*tracker.PeerInfo
	running      bool
	mux          sync.Mutex

	logger *logger.Logger
}

func New(
	announce string,
	announceList [][]string,
	infoHash []byte,
	listenPort uint16,
	left int64,
	complete bool,
	peerID []byte,
) *TrackerList {
	var newAnnounceList [][]string
	if announce != "" {
		newAnnounceList = append([][]string{{announce}}, announceList...)
	} else {
		newAnnounceList = announceList
	}
	return &TrackerList{
		announceList: newAnnounceList,
		infoHash:     infoHash,
		port:         listenPort,
		downloaded:   0,
		uploaded:     0,
		left:         left,
		complete:     complete,
		peerID:       peerID,
		Seeders:      0,
		Leechers:     0,
		Peers:        make([]*tracker.PeerInfo, 0),
		running:      false,
		Trackers:     make([]*tracker.Tracker, 0),
		logger:       logger.New(logger.DEBUG, "TrackerList"),
	}
}

func (tl *TrackerList) Start() error {
	if len(tl.announceList) == 0 {
		return errors.New("no trackers in list")
	}
	for _, list := range tl.announceList {
		for _, url := range list {
			newTracker := tracker.New(
				url,
				tl.infoHash,
				tl.port,
				tl.downloaded,
				tl.uploaded,
				tl.left,
				tl.peerID,
			)
			tl.Trackers = append(tl.Trackers, newTracker)
			go newTracker.Start()
			tl.logger.Debug(fmt.Sprintf("Started tracker: %s\n", newTracker.TrackerUrl))
		}
	}
	return nil
}

func (tl *TrackerList) Stop() {
	tl.logger.Debug("Stopping all trackers...")
	tl.running = false
	for _, tr := range tl.Trackers {
		go tr.Stop()
	}
	for _, tr := range tl.Trackers {
		if !tr.Stopped {
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func (tl *TrackerList) SetComplete() {
	tl.complete = true
	for _, tr := range tl.Trackers {
		err := tr.Complete()
		if err != nil {
			tl.logger.Error(fmt.Sprintf("Error setting complete on tracker(%s): %v\n", tr.TrackerUrl, err))
		}
	}
}

func (tl *TrackerList) GetPeers() []*tracker.PeerInfo {
	for _, tr := range tl.Trackers {
		for _, newPeer := range tr.Peers {
			found := false
			for _, oldPeer := range tl.Peers {
				if oldPeer.IP == newPeer.IP && oldPeer.Port == newPeer.Port {
					found = true
					break
				}
			}
			if !found {
				tl.Peers = append(tl.Peers, newPeer)
			}
		}
	}
	return tl.Peers
}

func (tl *TrackerList) SetDownloaded(downloadedBytes int64) {
	tl.downloaded = downloadedBytes
	for _, tr := range tl.Trackers {
		tr.SetDownloaded(downloadedBytes)
	}
}

func (tl *TrackerList) SetUploaded(uploadedBytes int64) {
	tl.uploaded = uploadedBytes
	for _, tr := range tl.Trackers {
		tr.SetUploaded(uploadedBytes)
	}
}

func (tl *TrackerList) SetLeft(leftBytes int64) {
	tl.left = leftBytes
	for _, tr := range tl.Trackers {
		tr.SetLeft(leftBytes)
	}
}

func (tl *TrackerList) GetStatuses() [][]string {
	statuses := make([][]string, 0)
	for _, tr := range tl.Trackers {
		if tr.TrackerError == nil {
			statuses = append(statuses, []string{tr.TrackerUrl, "working"})
		} else {
			statuses = append(statuses, []string{tr.TrackerUrl, fmt.Sprintf("%v", tr.TrackerError)})
		}
	}
	return statuses
}
