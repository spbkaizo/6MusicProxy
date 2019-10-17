// main
package main

import (
	"bytes"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/grafov/m3u8"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var sourceurl = "http://as-hls-uk-live.akamaized.net/pool_904/live/uk/bbc_6music/bbc_6music.isml/bbc_6music-audio%3d320000.norewind.m3u8"
var client = &http.Client{}
var tracks []string
var useragent = "VLC/3.0.8 LibVLC/3.0.8"
var tbytes int64
var seqnumber uint64
var ourseqnumber int

//var oursegmentnum uint64
var starttime time.Time

//var datadir = "hls/"
var port string = "8888"
var statefile = "/var/db/sequence.dat"

var currentplist []byte
var buffers = make(map[string][]byte)

/*
type Station int

const (
	BBC_Radio_1 Station = 1
	BBC_Radio_2 Station = 2
	BBC_6_Music Station = 6
)

func (station Station) URL() string {
	stations := make(map[int]string)
	stations[BBC_Radio_1] = "http://a.files.bbci.co.uk/media/live/manifesto/audio/simulcast/hls/uk/sbr_high/ak/bbc_radio_one.m3u8"
	stations[BBC_Radio_2] = "http://a.files.bbci.co.uk/media/live/manifesto/audio/simulcast/hls/uk/sbr_high/ak/bbc_radio_two.m3u8"
	// return the name of a Weekday
	// constant from the names array
	// above.
	return names[station]
}
*/

var (
	once      sync.Once
	netClient *http.Client
)

// Create a single client connection, use keepalive to hold open
// and reduce handshake times...
func newNetClient() *http.Client {
	once.Do(func() {
		var netTransport = &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 2 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 2 * time.Second,
			DisableCompression:  true,
		}
		netClient = &http.Client{
			Timeout:   time.Second * 2,
			Transport: netTransport,
		}
	})

	return netClient
}

// Make log messages print out prettier bytes info...
func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func getContent(u *url.URL) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		log.Printf("ERROR:  %v", err)
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR:  %v", err)
	}
	if resp.StatusCode != 200 {
		log.Printf("Received HTTP %v for %v\n", resp.StatusCode, u.String())
		return nil, err
	}
	//log.Printf("DEBUG: Server Headers: %v", resp.Header)
	tbytes = tbytes + resp.ContentLength
	return resp.Body, err
}

func absolutize(rawurl string, u *url.URL) (uri *url.URL, err error) {
	//log.Printf("DEBUG: rawurl %v, u %v", rawurl, u.String())

	suburl := rawurl
	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}

	if rawurl == u.String() {
		return
	}

	if !uri.IsAbs() { // relative URI
		if rawurl[0] == '/' { // from the root
			suburl = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, rawurl)
		} else { // last element
			splitted := strings.Split(u.String(), "/")
			splitted[len(splitted)-1] = rawurl

			suburl = strings.Join(splitted, "/")
		}
	}

	suburl, err = url.QueryUnescape(suburl)
	if err != nil {
		return
	}

	uri, err = u.Parse(suburl)
	if err != nil {
		return
	}
	return
}

func getPlaylist(u *url.URL) {
	content, err := getContent(u)
	if err != nil {
		log.Fatal("cms9> " + err.Error())
	}
	playlist, listType, err := m3u8.DecodeFrom(content, true)
	if err != nil {
		log.Fatal("cms10> " + err.Error())
	}
	content.Close()

	if listType != m3u8.MEDIA && listType != m3u8.MASTER {
		log.Fatal("cms11> " + "Not a valid playlist")
		return
	}

	if listType == m3u8.MASTER {

		masterpl := playlist.(*m3u8.MasterPlaylist)
		for _, variant := range masterpl.Variants {
			if variant != nil {
				msURL, err := absolutize(variant.URI, u)
				if err != nil {
					log.Fatal("cms12> " + err.Error())
				}
				getPlaylist(msURL)
			}

		}
		return
	}

	if listType == m3u8.MEDIA {
		mediapl := playlist.(*m3u8.MediaPlaylist)
		newplist, err := m3u8.NewMediaPlaylist(mediapl.Count(), mediapl.Count())
		if err != nil {
			log.Printf("ERROR:  %v", err)
		}
		if mediapl.SeqNo > seqnumber {
			ourseqnumber++
			newplist.SeqNo = uint64(ourseqnumber)
			for _, segment := range mediapl.Segments {
				if segment != nil {
					msURL, err := absolutize(segment.URI, u)
					if err != nil {
						log.Fatal("cms15> " + err.Error())
					}
					seen := false
					//log.Printf("DEBUG: TRACKS: %v", tracks)
					for i, track := range tracks {
						/*
							u, err := url.Parse(msURL.String())
							if err != nil {
								log.Printf("ERROR: %v", err)
							}
						*/
						//log.Printf("DEBUG: Track in loop %v at pos %v", track, i)
						if len(tracks) > 12 {
							//log.Printf("DEBUG len(tracks) is %v", len(tracks))
							u, err := url.Parse(tracks[0])
							if err != nil {
								log.Printf("ERROR:  %v", err)
							}
							file := path.Base(u.Path)
							tracks = append(tracks[:i], tracks[i+1:]...) // keep track of tracks...
							delete(buffers, file)
						}
						//if track == msURL.String() {
						//log.Printf("DEBUG: track %v, msurls %v", track, path.Base(msURL.String()))
						if track == path.Base(msURL.String()) {
							seen = true
						}
					}
					if seen == false {
						//oursegmentnum++ // increment
						start := time.Now()
						//tracks = append(tracks, msURL.String())
						//download(msURL)
						newbuf := new(bytes.Buffer)
						content, err := getContent(msURL)
						newbuf.ReadFrom(content)
						content.Close() // done with it
						buffers[path.Base(msURL.Path)] = newbuf.Bytes()
						u, err := url.Parse(msURL.String())
						if err != nil {
							log.Printf("Error: %v", err)
						}
						elapsed := time.Since(start)
						uptime := time.Since(starttime)
						log.Printf("TRACK:  %v downloaded in %v, data total: %v, uptime: %v", path.Base(u.Path), elapsed.Truncate(time.Millisecond), ByteCountSI(tbytes), uptime.Truncate(time.Second))
						if err != nil {
							log.Printf("ERROR:  %v", err)
						}
						tracks = append(tracks, path.Base(u.Path))
						//newplist.Append(path.Base(u.Path), segment.Duration, "foo")
						//log.Printf("%v", newplist)
					}
					// copy the items to our new playlist, copy details from original too.
					//newplist.Append(path.Base(msURL.String()), segment.Duration, "foo")
					newsegment := m3u8.MediaSegment{
						Title:    "Kaizo HLS Relay",
						URI:      path.Base(msURL.String()),
						Duration: segment.Duration}
					//log.Printf("DEBUG: %v", newsegment)
					err = newplist.AppendSegment(&newsegment)
					if err != nil {
						log.Printf("ERROR:  %v", err)
					}
				}
			}
			seqnumber = mediapl.SeqNo
			// store playlist in memory for clients
			//currentplist = m3u8.Playlist(mediapl).Encode().Bytes()
			currentplist = m3u8.Playlist(newplist).Encode().Bytes()
			state := []byte(strconv.Itoa(ourseqnumber))
			err := ioutil.WriteFile(statefile, state, 0644)
			if err != nil {
				log.Printf("ERROR:  Writing statefile %v (%v)", statefile, err)
			}
		}
	}
}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Powered-By", "Golang!")
	//fmt.Fprintf(w, string(currentplist))
	fmt.Fprintf(w, "nothing to see here...")
}

func fileHandler(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	w.Header().Set("X-Powered-By", "Golang!")
	path := filepath.Base(req.URL.Path)
	//log.Printf("REQ for %v", path)
	if strings.Contains(path, "m3u8") {
		// application/x-mpegURL
		w.Header().Set("Content-Type", "application/x-mpegURL")
		fmt.Fprintf(w, string(currentplist))
	} else if data, ok := buffers[path]; ok {
		// video/MP2T
		w.Header().Set("Content-Type", "video/MP2T")
		_, err := w.Write(data)
		if err != nil {
			log.Printf("ERROR:  %v", err)
		}
	} else {
		fmt.Fprintf(w, "meep!")
	}
	//log.Printf("DEBUG: Req - %v", req)
	log.Printf("REMOTE: Client from %v served %v in %v [%v]", req.RemoteAddr, path, time.Since(start), req.Header.Get("User-Agent"))
}

func main() {
	starttime = time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if !strings.HasPrefix(sourceurl, "http") {
		log.Fatal("cms17> " + "Playlist URL must begin with http/https")
	}

	target, err := url.Parse(sourceurl)
	if err != nil {
		log.Fatal("cms18> " + err.Error())
	}

	if _, err := os.Stat(statefile); os.IsNotExist(err) {
		log.Printf("WARNING: Could not load current state from file %v (%v)", statefile, err)
	} else {
		oldstate, err := ioutil.ReadFile(statefile)
		if err != nil {
			log.Printf("ERROR:  State file %v exists, but cannot open (%v)", statefile, err)
		}
		ourseqnumber, err = strconv.Atoi(string(oldstate))
		if err != nil {
			log.Printf("ERROR:  %v", err)
		}
	}
	timer := time.NewTicker(500 * time.Millisecond)
	go func() {
		for _ = range timer.C {
			getPlaylist(target)
		}
	}()

	// debugging
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/{filename}", fileHandler)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	srv.SetKeepAlivesEnabled(true)
	log.Fatal(srv.ListenAndServe())
}
