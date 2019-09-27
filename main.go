// main
package main

import (
	"bytes"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/grafov/m3u8"
	"io"
	//"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	//"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var sourceurl = "http://as-hls-uk-live.akamaized.net/pool_904/live/uk/bbc_6music/bbc_6music.isml/bbc_6music-audio%3d320000.norewind.m3u8"
var client = &http.Client{}
var tracks []string
var useragent = "VLC/3.0.8 LibVLC/3.0.8"
var tbytes uint64
var seqnumber uint64
var starttime time.Time

//var datadir = "hls/"
var port string = "8888"
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
func ByteCountSI(b uint64) string {
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
		log.Printf("ERROR: %v", err)
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("ERROR: %v", err)
	}
	if resp.StatusCode != 200 {
		log.Printf("Received HTTP %v for %v\n", resp.StatusCode, u.String())
		return nil, err
	}
	//log.Printf("DEBUG: Server Headers: %v", resp.Header)
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
	//log.Printf("DEBUG: uri = %v", uri.String())
	return
}

/*
func writePlaylist(u *url.URL, mpl m3u8.Playlist) {
	fileName := filepath.Base(u.Path)
	// Write to a temp file, to avoid the delay of the
	// m3u8 encoder writing to the main playlist file.
	// This occasionally leads to a race condition with clients
	// otherwise.
	tmpfile, err := ioutil.TempFile(datadir, fileName+"-")
	if err != nil {
		log.Fatal(err)
	}

	//defer os.Remove(tmpfile.Name()) // clean up
	//encoderstart := time.Now() // DEBUG
	if _, err := tmpfile.Write(mpl.Encode().Bytes()); err != nil {
		log.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		log.Fatal(err)
	}
	//log.Printf("DEBUG: File %v written in %v", tmpfile.Name(), time.Since(encoderstart))
	// Now move the tmp file over the original
	//start := time.Now()
	err = os.Rename(tmpfile.Name(), datadir+fileName)
	if err != nil {
		log.Printf("ERROR: %v", err)
	}
	//log.Printf("DEBUG: File %v moved in %v.", tmpfile.Name(), time.Since(start))
}
*/

func download(u *url.URL) {
	fileName := path.Base(u.Path)

	/*out, err := os.Create(datadir + fileName)
	if err != nil {
		log.Fatal("cms5> " + err.Error())
	}
	defer out.Close()
	*/

	content, err := getContent(u)
	if err != nil {
		log.Print("cms6> " + err.Error())
		//continue
	}
	defer content.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(content)
	buffers[fileName] = buf.Bytes()

	// FIX BYTE COUNT
	/*
		size, err := io.Copy(out, content)
		tbytes = tbytes + uint64(size)
		if err != nil {
			log.Print("cms7> " + err.Error() + "Failed to download " + fileName + "\n")
		}
	*/

}

func getPlaylist(u *url.URL) {
	//start := time.Now()

	//cache := lru.New(64)

	content, err := getContent(u)
	if err != nil {
		log.Fatal("cms9> " + err.Error())
	}

	//elapsed := time.Since(start)
	//log.Printf("PLAYLIST: %v completed in %v", path.Base(u.Path), elapsed)

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
				//log.Print("cms13> "+"Downloaded chunklist number ", k+1, "\n\n")
				//break
			}

		}
		//writePlaylist(u, m3u8.Playlist(masterpl))
		return
	}

	if listType == m3u8.MEDIA {
		mediapl := playlist.(*m3u8.MediaPlaylist)
		//log.Printf("DEBUG: Playlist %v", mediapl.SeqNo)
		if mediapl.SeqNo > seqnumber {
			for _, segment := range mediapl.Segments {
				if segment != nil {

					msURL, err := absolutize(segment.URI, u)
					if err != nil {
						log.Fatal("cms15> " + err.Error())
					}

					//_, hit := cache.Get(msURL.String())
					//if !hit {
					seen := false
					for _, track := range tracks {
						if len(tracks) > 12 {
							u, err := url.Parse(tracks[0])
							if err != nil {
								log.Printf("ERROR: %v", err)
							}
							//log.Printf("Tracks[0] is %v", tracks[0])
							// 2019/09/23 19:07:56 u.Path = /pool_904/live/uk/bbc_6music/bbc_6music.isml/bbc_6music-audio=320000-245197198.ts
							//start := time.Now()
							file := path.Base(u.Path)
							//file := strings.TrimPrefix(u.Path, "/pool_904/live/uk/bbc_6music/bbc_6music.isml/")
							/*
									err = os.Remove(datadir + file)
								if err != nil {
									log.Printf("Error removing stale file %v (%v)", file, err)
								} else {
									tracks = append(tracks[:0], tracks[0+1:]...)
								}
							*/
							tracks = append(tracks[:0], tracks[0+1:]...) // keep track of tracks...
							// map
							delete(buffers, file)
							for k, _ := range buffers {
								log.Printf("DEBUG: BUFFER %v exists", k)
							}
							//elapsed := time.Since(start)
							//log.Printf("TRACK: %v deleted from disk", file)
						}

						if track == msURL.String() {
							//log.Printf("Already Seen %v", msURL.String())
							seen = true
							break
						}
					}
					if seen == false {
						start := time.Now()
						tracks = append(tracks, msURL.String())
						download(msURL)
						u, err := url.Parse(msURL.String())
						//log.Printf("DEBUG %v", u.String())
						if err != nil {
							log.Printf("Error: %v", err)
						}
						//log.Printf("TRACK: %v added to cache/download", msURL.String())
						elapsed := time.Since(start)
						uptime := time.Since(starttime)
						log.Printf("TRACK: %v downloaded in %v, data total: %v, uptime: %v", path.Base(u.Path), elapsed.Truncate(time.Millisecond), ByteCountSI(tbytes), uptime.Truncate(time.Second))
						if err != nil {
							log.Printf("ERROR: %v", err)
						}
					}
					//tracks = append(tracks, msURL.String())
					//download(msURL)
					//log.Printf("TRACK: %v added to cache/download", msURL.String())
					//log.Printf("Tracks: %v", tracks)
					//cache.Add(msURL.String(), nil)
					//download(msURL)
					//}

				}
			}
			//log.Printf("DEBUG writing playlist for seqnumber %v (was %v", mediapl.SeqNo, seqnumber)
			seqnumber = mediapl.SeqNo
			// handle 'u' ?
			//log.Printf("DEBUG : %v", u.String())
			//writePlaylist(u, m3u8.Playlist(mediapl))
			currentplist = m3u8.Playlist(mediapl).Encode().Bytes()
			//log.Printf("PLAYLIST: SeqNo %v written to disk", seqnumber)
		}
	}
}

func indexHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Powered-By", "Golang!")
	// currentplist
	fmt.Fprintf(w, string(currentplist))
}

func fileHandler(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	w.Header().Set("X-Powered-By", "Golang!")
	//log.Printf("DEBUG: req.URL.Path=%v", req.URL.Path)
	// currentplist
	//fmt.Fprintf(w, "meep!")
	//w.Write(currentplist)
	path := filepath.Base(req.URL.Path)
	if data, ok := buffers[path]; ok {
		log.Printf("buffer found")
		_, err := w.Write(data)
		if err != nil {
			log.Printf("ERROR: %v", err)
		}
	} else {
		log.Printf("buffer not found, looking for %v", path)
		fmt.Fprintf(w, string(currentplist))
	}
	/*
		_, err = os.Stat(datadir + path)
		if os.IsNotExist(err) {
			fmt.Fprintf(w, string(currentplist))
			return
		} else if err != nil {
			// if we got an error (that wasn't that the file doesn't exist) stating the
			// file, return a 500 internal server error and stop
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		content, err := ioutil.ReadFile(datadir + path)
		if err != nil {
			log.Printf("ERROR: %v")
		}
		_, err = w.Write(content)
		if err != nil {
			log.Printf("ERROR: %v")
		}
	*/
	log.Printf("REMOTE: Client from %v served %v in %v", req.RemoteAddr, path, time.Since(start))
}

func main() {
	starttime = time.Now()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	/*
		err := os.MkdirAll(datadir, 0755)
		if err != nil {
			log.Printf("ERROR: Creating %v data directory (%v)", datadir, err)
		}
		// cleanup all stale files matching bbc_6music-audio=320000*
		files, err := filepath.Glob(datadir + "bbc_6music-audio=320000*")
		if err != nil {
			panic(err)
		}
		for _, f := range files {
			if err := os.Remove(f); err != nil {
				panic(err)
			}
			log.Printf("INFO: Deleting stale media file %v", f)
		}
	*/
	//log.Printf(Station(1))
	if !strings.HasPrefix(sourceurl, "http") {
		log.Fatal("cms17> " + "Playlist URL must begin with http/https")
	}

	target, err := url.Parse(sourceurl)
	if err != nil {
		log.Fatal("cms18> " + err.Error())
	}
	timer := time.NewTicker(1337 * time.Millisecond)
	go func() {
		for _ = range timer.C {
			getPlaylist(target)
		}
	}()
	/*
		for {
			getPlaylist(target)
			time.Sleep(1337 * time.Millisecond)
		}
	*/

	//log.Fatal(http.ListenAndServe(":"+port, http.FileServer(http.Dir(datadir))))
	//http.HandleFunc("/", indexHandler)
	//http.Handle("/hls", http.FileServer(http.Dir(datadir)))
	//log.Fatal(http.ListenAndServe(":"+port, nil))

	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/{filename}", fileHandler)

	srv := &http.Server{
		Handler: r,
		Addr:    ":" + port,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())

}
