package HLS

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"time"

	. "github.com/langhuihui/monibuca/monica"
	"github.com/quangngotan95/go-m3u8/m3u8"
)

type HLS struct {
	TS
	HLSInfo
	TsHead      http.Header
	SaveContext context.Context
}
type HLSInfo struct {
	Video  M3u8Info
	Audio  M3u8Info
	TSInfo *TSInfo
}
type M3u8Info struct {
	Req       *http.Request `json:"-"`
	M3U8Count int
	TSCount   int
	LastM3u8  string
	M3u8Info  []TSCost
}
type TSCost struct {
	DownloadCost int
	DecodeCost   int
	BufferLength int
}

func readM3U8(res *http.Response) (playlist *m3u8.Playlist, err error) {
	var reader io.Reader = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(reader)
	}
	if err == nil {
		playlist, err = m3u8.Read(reader)
	}
	if err != nil {
		log.Printf("readM3U8 error:%s", err.Error())
	}
	return
}
func (p *HLS) run(info *M3u8Info) {
	client := http.Client{Timeout: time.Second * 5}
	sequence := 0
	lastTs := make(map[string]bool)
	resp, err := client.Do(info.Req)
	defer func() {
		log.Printf("hls %s exit:%v", p.StreamPath, err)
		p.Cancel()
	}()
	for ; err == nil && p.Err() == nil; resp, err = client.Do(info.Req) {
		if playlist, err := readM3U8(resp); err == nil {

			info.LastM3u8 = playlist.String()
			//if !playlist.Live {
			//	log.Println(p.LastM3u8)
			//	return
			//}
			if playlist.Sequence <= sequence {
				log.Printf("same sequence:%d,max:%d", playlist.Sequence, sequence)
				time.Sleep(time.Second)
				continue
			}
			info.M3U8Count++
			sequence = playlist.Sequence
			thisTs := make(map[string]bool)
			tsItems := make([]*m3u8.SegmentItem, 0)
			discontinuity := false
			for _, item := range playlist.Items {
				switch v := item.(type) {
				case *m3u8.DiscontinuityItem:
					discontinuity = true
				case *m3u8.SegmentItem:
					thisTs[v.Segment] = true
					if _, ok := lastTs[v.Segment]; ok && !discontinuity {
						continue
					}
					tsItems = append(tsItems, v)
				}
			}
			lastTs = thisTs
			if len(tsItems) > 3 {
				tsItems = tsItems[len(tsItems)-3:]
			}
			info.M3u8Info = nil
			for _, v := range tsItems {
				tsCost := TSCost{}
				tsUrl, _ := info.Req.URL.Parse(v.Segment)
				tsReq, _ := http.NewRequest("GET", tsUrl.String(), nil)
				tsReq.Header = p.TsHead
				t1 := time.Now()
				if tsRes, err := client.Do(tsReq); err == nil && p.Err() == nil {
					info.TSCount++
					if body, err := ioutil.ReadAll(tsRes.Body); err == nil && p.Err() == nil {
						tsCost.DownloadCost = int(time.Since(t1) / time.Millisecond)
						if p.SaveContext != nil && p.SaveContext.Err() == nil {
							err = ioutil.WriteFile(filepath.Base(tsUrl.Path), body, 0666)
						}
						t1 = time.Now()
						beginLen := len(p.TsPesPktChan)
						if err = p.Feed(bytes.NewReader(body)); p.Err() != nil {
							close(p.TsPesPktChan)
						}
						tsCost.DecodeCost = int(time.Since(t1) / time.Millisecond)
						tsCost.BufferLength = len(p.TsPesPktChan)
						p.PesCount = tsCost.BufferLength - beginLen
					} else if err != nil {
						log.Printf("%s readTs:%v", p.StreamPath, err)
					}
				} else if err != nil {
					log.Printf("%s reqTs:%v", p.StreamPath, err)
				}
				info.M3u8Info = append(info.M3u8Info, tsCost)
			}

			time.Sleep(time.Second * time.Duration(playlist.Target) * 2)
		} else {
			log.Printf("%s readM3u8:%v", p.StreamPath, err)
			return
		}
	}
}
func (p *HLS) Publish(streamName string, publisher Publisher) (result bool) {
	if result = p.TS.Publish(streamName, publisher); result {
		p.HLSInfo.TSInfo = &p.TS.TSInfo
		go p.run(&p.HLSInfo.Video)
		if p.HLSInfo.Audio.Req != nil {
			go p.run(&p.HLSInfo.Audio)
		}
	}
	return
}
