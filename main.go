// main
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
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

	"github.com/gorilla/mux"
	"github.com/sevlyar/go-daemon"
	"github.com/spbkaizo/m3u8"
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

// ByteCountSI Make log messages print out prettier bytes info...
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
		return nil, err
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR:  %v", err)
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.Printf("Received HTTP %v for %v\n", resp.StatusCode, u.String())
		return nil, errors.New("Request was fine, but status code isn't 200")
		//return nil, err
	}
	//log.Printf("DEBUG: Server Headers: %v", resp.Header)

	tbytes = tbytes + resp.ContentLength
	/*
		log.Printf("DEBUG: Body: %#v", resp.Body)
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("DEBUG: Body Read Error: %v", err)
		}
		log.Printf("DEBUG: Body: %v", string(body))
	*/
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

func getPlaylist(u *url.URL) error {
	content, err := getContent(u)
	if err != nil {
		log.Printf("ERROR: Downloading Playlist (%vi)", err)
		return err
	}
	playlist, listType, err := m3u8.DecodeFrom(content, false)
	//log.Printf("DEBUG: %#v, %#v", playlist, listType)
	if err != nil {
		//log.Fatal("cms10> " + err.Error())
		log.Printf("ERROR: Decoding Playlist (%v)", err)
		return err
	}
	content.Close()

	if listType != m3u8.MEDIA && listType != m3u8.MASTER {
		//log.Fatal("cms11> " + "Not a valid playlist")
		log.Printf("ERROR: Not a valid playlist")
		return err
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
		return nil
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
						if err != nil {
							log.Printf("ERROR: Getting content from %v", msURL)
							return nil
						}
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
	return nil
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
	daemonFlag := flag.Bool("d", false, "Run as a daemon")
	pidFile := flag.String("pidfile", "/var/run/6music.pid", "Path to the PID file")
	logFile := flag.String("logfile", "/var/log/6music.log", "Path to the log file")
	workDir := flag.String("workdir", "/var/empty/", "Working directory")
	umask := flag.Int("umask", 022, "Umask for the daemon")
	flag.Parse()

	// Override with environment variables if set
	if envPidFile := os.Getenv("PID_FILE"); envPidFile != "" {
		*pidFile = envPidFile
	}
	if envLogFile := os.Getenv("LOG_FILE"); envLogFile != "" {
		*logFile = envLogFile
	}
	if envWorkDir := os.Getenv("WORK_DIR"); envWorkDir != "" {
		*workDir = envWorkDir
	}
	if envUmask := os.Getenv("UMASK"); envUmask != "" {
		umaskVal, err := strconv.ParseInt(envUmask, 8, 0)
		if err == nil {
			*umask = int(umaskVal)
		}
	}

	var cntxt *daemon.Context
	if *daemonFlag {
		cntxt = &daemon.Context{
			PidFileName: *pidFile,
			PidFilePerm: 0644,
			LogFileName: *logFile,
			LogFilePerm: 0644,
			WorkDir:     *workDir,
			Umask:       int(*umask),
		}

		d, err := cntxt.Reborn()
		if err != nil {
			log.Fatal("Unable to run: ", err)
		}
		if d != nil {
			return
		}
		defer cntxt.Release()
	}

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
		err := os.WriteFile(statefile, []byte("1"), 0644)
		if err != nil {
			log.Printf("FATAL: Could not create statefile %v (%v)", statefile, err)
			log.Printf("INFO: Please ensure file exists and has the correct owner, or the user running this has permissions to write to the file/directory.")
		}
		ourseqnumber = 1
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
	timer := time.NewTicker(5000 * time.Millisecond)
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
	r.HandleFunc("/6MusicProxy/", indexHandler)
	r.HandleFunc("/6MusicProxy/{filename}", fileHandler)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	srv.SetKeepAlivesEnabled(true)
	log.Fatal(srv.ListenAndServe())
}
