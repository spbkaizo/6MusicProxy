// main
package main

import (
	"fmt"
	"github.com/grafov/m3u8"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

var client = &http.Client{}
var tracks []string
var useragent = "VLC/3.0.8 LibVLC/3.0.8"
var tbytes uint64

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

func writePlaylist(u *url.URL, mpl m3u8.Playlist) {
	fileName := path.Base(u.Path)
	out, err := os.Create(OUT_PATH + fileName)
	if err != nil {
		log.Fatal("cms3> " + err.Error())
	}
	defer out.Close()

	_, err = mpl.Encode().WriteTo(out)
	if err != nil {
		log.Fatal("cms4> " + err.Error())
	}
}

func download(u *url.URL) {
	fileName := path.Base(u.Path)

	out, err := os.Create(OUT_PATH + fileName)
	if err != nil {
		log.Fatal("cms5> " + err.Error())
	}
	defer out.Close()

	content, err := getContent(u)
	if err != nil {
		log.Print("cms6> " + err.Error())
		//continue
	}
	defer content.Close()

	size, err := io.Copy(out, content)
	tbytes = tbytes + uint64(size)
	if err != nil {
		log.Print("cms7> " + err.Error() + "Failed to download " + fileName + "\n")
	}
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
		writePlaylist(u, m3u8.Playlist(masterpl))
		return
	}

	if listType == m3u8.MEDIA {
		mediapl := playlist.(*m3u8.MediaPlaylist)
		for _, segment := range mediapl.Segments {
			if segment != nil {

				msURL, err := absolutize(segment.URI, u)
				if err != nil {
					log.Fatal("cms15> " + err.Error())
				}
				//log.Printf("new url, %v\n", msURL.String())

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
						err = os.Remove(file)
						if err != nil {
							log.Printf("Error removing stale file %v (%v)", file, err)
						} else {
							tracks = append(tracks[:0], tracks[0+1:]...)
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
					if err != nil {
						log.Printf("Error: %v", err)
					}
					elapsed := time.Since(start)
					//log.Printf("TRACK: %v added to cache/download", msURL.String())
					log.Printf("TRACK: %v downloaded in %v, data total: %v", path.Base(u.Path), elapsed, ByteCountSI(tbytes))
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

		writePlaylist(u, m3u8.Playlist(mediapl))
		//log.Print("cms16> "+"Downloaded Media Playlist: ", path.Base(u.Path))

		//time.Sleep(time.Duration(int64(mediapl.TargetDuration)) * time.Second)

	}

	//time.Sleep(time.Duration(11) * time.Second)

}

var OUT_PATH string = "./"

var IN_URL string = "http://as-hls-uk-live.akamaized.net/pool_904/live/uk/bbc_6music/bbc_6music.isml/bbc_6music-audio%3d320000.norewind.m3u8"

//var IN_URL string = "http://a.files.bbci.co.uk/media/live/manifesto/audio/simulcast/hls/uk/sbr_high/ak/bbc_6music.m3u8"

//var IN_URL string = "http://makombo.org/cast/media/cmshlstest/master.m3u8"
//var IN_URL string = "http://makombo.org/cast/media/DevBytes%20Google%20Cast%20SDK_withGDLintro_Apple_HLS_h264_SF_16x9_720p/DevBytes%20Google%20Cast%20SDK_withGDLintro_Apple_HLS_h264_SF_16x9_720p.m3u8"
//var IN_URL string = "http://makombo.org/cast/media/DevBytes%20Google%20Cast%20SDK_withGDLintro_Apple_HLS_h264_SF_16x9_720p/stream-2-229952/index.m3u8"
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	//log.Printf(Station(1))
	if !strings.HasPrefix(IN_URL, "http") {
		log.Fatal("cms17> " + "Playlist URL must begin with http/https")
	}

	fmt.Print("\n\n\n")

	theURL, err := url.Parse(IN_URL)
	if err != nil {
		log.Fatal("cms18> " + err.Error())
	}
	for {
		getPlaylist(theURL)
		time.Sleep(1 * time.Second)
	}

}
