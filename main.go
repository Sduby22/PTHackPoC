package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/lyc8503/ptcheat/util"
)

var FAKE_SPEED = 512 * 1024 // 512KB/s
var port int = 17673
var peerId string = util.RandomPeerId()
var key string = util.RandomKey()

type InfoHash struct {
	InfoHash        string
	InfoHashEncoded string
	Size            int64
	Downloaded      int64
	Uploaded        int64
	Left            int64
	MaxDownload     int64
	TrackerUrl      string
	CachedPeers     []byte
}

var infoHashMap = make(map[string]InfoHash)

func makeInfoHash(infoHash string, initSize int64, trackerUrl string) InfoHash {
	maxDownload := rand.Int63n(5120*1024-512*1024) + 512*1024
	infoHashEncoded := url.QueryEscape(infoHash)

	return InfoHash{
		InfoHash:        infoHash,
		InfoHashEncoded: infoHashEncoded,
		Size:            initSize,
		Downloaded:      0,
		Uploaded:        0,
		Left:            initSize,
		MaxDownload:     maxDownload,
		TrackerUrl:      trackerUrl,
	}
}

type TrackerEvent string

const (
	TrackerEventStarted   TrackerEvent = "started"
	TrackerEventStopped   TrackerEvent = "stopped"
	TrackerEventCompleted TrackerEvent = "completed"
	TrackerEventEmpty     TrackerEvent = ""
)

func trackerReqUrl(infoHashObj InfoHash, event TrackerEvent) string {
	return fmt.Sprintf("%s&info_hash=%s&peer_id=%s&port=%d&uploaded=0&downloaded=%d&left=%d&corrupt=0&key=%s"+
		"&event=%s&numwant=200&compact=1&no_peer_id=1&supportcrypto=1&redundant=0",
		infoHashObj.TrackerUrl, infoHashObj.InfoHashEncoded, peerId, port, infoHashObj.Downloaded, infoHashObj.Left, key, event)
}

func requestTrackerStop(infoHashObj InfoHash) {
	reqURL := trackerReqUrl(infoHashObj, TrackerEventStopped)
	fmt.Println("Requesting:", reqURL)
}

func requestTrackerInterval(trackerUrl string, infoHashObj InfoHash) {
	// update downloaded and left
	fakeRatio := rand.Float32()*0.2 - 0.1 + 1.0
	fakeDownloaded := int64(float32(FAKE_SPEED) * fakeRatio)

	infoHashObj.Downloaded += fakeDownloaded
	infoHashObj.Left -= fakeDownloaded

	reqURL := trackerReqUrl(infoHashObj, TrackerEventEmpty)
	fmt.Println("Requesting:", reqURL)
}

func requestTrackerStart(trackerUrl string, infoHash string, initSize int64) {
	if _, ok := infoHashMap[infoHash]; ok {
		return
	}

	infoHashObj := makeInfoHash(infoHash, initSize, trackerUrl)
	reqURL := trackerReqUrl(infoHashObj, TrackerEventStarted)

	fmt.Println("Requesting:", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("User-Agent", "qBittorrent/5.0.2")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Connection", "close")

	// TODO: fake TLS handshake fingerprint
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	reader, err := gzip.NewReader(resp.Body)
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		panic(err)
	}

	infoHashObj.CachedPeers = body
	infoHashMap[infoHash] = infoHashObj
}

func localFakeTrackerHandler(w http.ResponseWriter, r *http.Request) {
	infoHash := r.URL.Query().Get("info_hash")
	fmt.Println("received request: ", infoHash)

	if infoHashObj, ok := infoHashMap[infoHash]; ok {
		w.Write(infoHashObj.CachedPeers)
	} else {
		event := r.URL.Query().Get("event")
		origTracker := r.URL.Query().Get("orig_tracker")
		totalSize, err := strconv.ParseInt(r.URL.Query().Get("total_size"), 10, 64)
		if err != nil {
			panic(err)
		}
		if event == string(TrackerEventStarted) {
			fmt.Println("requesting tracker start: ", origTracker, infoHash, totalSize)
			requestTrackerStart(origTracker, infoHash, totalSize)
		}

		infoHashObj := infoHashMap[infoHash]
		w.Write(infoHashObj.CachedPeers)
	}
}

func main() {
	// spam peer
	if len(os.Args) >= 2 {
		util.ConnectPeer(os.Args[1], os.Args[2])
		return
	}

	// Below is the main logic to process *.torrent files
	// TODO: maybe generate a fixed port number on first run
	// ideally use a port number that is identical to the one in your BT client

	// List *.torrent files
	files, err := os.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for i := 0; i < len(files); i++ {
		if strings.HasSuffix(files[i].Name(), ".torrent") {
			if strings.HasPrefix(files[i].Name(), "FREE_") {
				fmt.Println("skipping already processed torrent: ", files[i].Name())
				continue
			}

			fmt.Println("processing: ", files[i].Name())
			_, hash, leftSize, err := util.ParseAndRegenerateTorrent(files[i].Name(), "http://127.0.0.1:1088/announce")
			if err != nil {
				fmt.Println("Error: ", err)
				continue
			}
			fmt.Printf("info_hash: %s, size: %d\n", hash, leftSize)
		}
	}

	http.HandleFunc("/announce", localFakeTrackerHandler)

	fmt.Println("Starting server at port 1088")
	if err := http.ListenAndServe(":1088", nil); err != nil {
		panic(err)
	}
}
