package hls

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/quangngotan95/go-m3u8/m3u8"
	"m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

// Plugin-specific progress step names for HLS
const (
	StepM3U8Fetch  pkg.StepName = "m3u8_fetch"
	StepM3U8Parse  pkg.StepName = "parse" // hls playlist parse
	StepTsDownload pkg.StepName = "ts_download"
)

// Fixed progress steps for HLS pull workflow
var hlsPullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing stream"},
	{Name: StepM3U8Fetch, Description: "Fetching M3U8 playlist"},
	{Name: StepM3U8Parse, Description: "Parsing M3U8 playlist"},
	{Name: StepTsDownload, Description: "Downloading TS segments"},
	{Name: pkg.StepStreaming, Description: "Processing and streaming"},
}

func NewPuller(conf config.Pull) m7s.IPuller {
	return &Puller{}
}

type Puller struct {
	task.Task
	PullJob     m7s.PullJob
	Video       M3u8Info
	Audio       M3u8Info
	TsHead      http.Header     `json:"-" yaml:"-"` //用于提供cookie等特殊身份的http头
	SaveContext context.Context `json:"-" yaml:"-"` //用来保存ts文件到服务器
	memoryTs    sync.Map
}

func (p *Puller) GetPullJob() *m7s.PullJob {
	return &p.PullJob
}

func (p *Puller) GetTs(key string) (any, bool) {
	return p.memoryTs.Load(key)
}

func (p *Puller) Start() (err error) {
	// Initialize progress tracking for pull operations
	p.PullJob.SetProgressStepsDefs(hlsPullSteps)

	if err = p.PullJob.Publish(); err != nil {
		p.PullJob.Fail(err.Error())
		return
	}

	p.PullJob.GoToStepConst(StepM3U8Fetch)

	p.PullJob.Publisher.Speed = 1
	if p.PullJob.PublishConfig.RelayMode != config.RelayModeRemux {
		MemoryTs.Store(p.PullJob.StreamPath, p)
	}
	return
}

func (p *Puller) Dispose() {
	if p.PullJob.PublishConfig.RelayMode == config.RelayModeRelay {
		MemoryTs.Delete(p.PullJob.StreamPath)
	}
}

func (p *Puller) Run() (err error) {
	p.Video.Req, err = http.NewRequest("GET", p.PullJob.RemoteURL, nil)
	if err != nil {
		return
	}
	p.Video.Req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36 Edg/138.0.0.0 Monibuca/5.0")
	return p.pull(&p.Video)
}

func (p *Puller) pull(info *M3u8Info) (err error) {
	//请求失败自动退出
	req := info.Req.WithContext(p.Context)
	client := p.PullJob.HTTPClient
	sequence := -1
	lastTs := make(map[string]bool)
	tsRing := util.NewRing[string](6)
	var tsReader *mpegts.MpegTsStream
	if p.PullJob.PublishConfig.RelayMode != config.RelayModeRelay {
		tsReader = &mpegts.MpegTsStream{
			Allocator: util.NewScalableMemoryAllocator(1 << util.MinPowerOf2),
		}
		tsReader.Publisher = p.PullJob.Publisher
		defer tsReader.Allocator.Recycle()
	}
	var maxResolution *m3u8.PlaylistItem
	for errcount := 0; err == nil; err = p.Err() {
		p.Debug("pull m3u8", "url", req.URL.String())
		resp, err1 := client.Do(req)
		if err1 != nil {
			return err1
		}
		req = resp.Request
		if playlist, err2 := readM3U8(resp); err2 == nil {
			errcount = 0
			info.LastM3u8 = playlist.String()
			//if !playlist.Live {
			//	log.Println(p.LastM3u8)
			//	return
			//}
			if playlist.Sequence <= sequence {
				p.Warn("same sequence", "sequence", playlist.Sequence, "max", sequence)
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
				case *m3u8.PlaylistItem:
					if (maxResolution == nil || maxResolution.Resolution != nil && (maxResolution.Resolution.Width < v.Resolution.Width || maxResolution.Resolution.Height < v.Resolution.Height)) || maxResolution.Bandwidth < v.Bandwidth {
						maxResolution = v
					}
				case *m3u8.DiscontinuityItem:
					discontinuity = true
				case *m3u8.SegmentItem:
					thisTs[v.Segment] = true
					if _, ok := lastTs[v.Segment]; ok && !discontinuity {
						continue
					}
					tsItems = append(tsItems, v)
				case *m3u8.MediaItem:
					if p.Audio.Req == nil {
						if url, err := req.URL.Parse(*v.URI); err == nil {
							newReq, _ := http.NewRequest("GET", url.String(), nil)
							newReq.Header = req.Header
							p.Audio.Req = newReq
							go p.pull(&p.Audio)
						}
					}
				}
			}
			if maxResolution != nil && len(tsItems) == 0 {
				if url, err := req.URL.Parse(maxResolution.URI); err == nil {
					if strings.HasSuffix(url.Path, ".m3u8") {
						p.Video.Req, _ = http.NewRequest("GET", url.String(), nil)
						p.Video.Req.Header = req.Header
						req = p.Video.Req
						sequence = -1
						continue
					}
				}
			}
			tsCount := len(tsItems)
			p.Debug("readM3U8", "sequence", sequence, "tscount", tsCount)
			lastTs = thisTs
			if tsCount > 3 {
				tsItems = tsItems[tsCount-3:]
			}
			var plBuffer util.Buffer
			relayPlayList := Playlist{
				Writer:         &plBuffer,
				Targetduration: playlist.Target,
				Sequence:       playlist.Sequence,
			}
			if p.PullJob.PublishConfig.RelayMode != config.RelayModeRemux {
				relayPlayList.Init()
			}
			var tsDownloaders = make([]*TSDownloader, len(tsItems))
			for i, v := range tsItems {
				if p.Err() != nil {
					return p.Err()
				}
				tsUrl, _ := info.Req.URL.Parse(v.Segment)
				tsReq, _ := http.NewRequestWithContext(p.Context, "GET", tsUrl.String(), nil)
				tsReq.Header = p.TsHead
				// t1 := time.Now()
				tsDownloaders[i] = &TSDownloader{
					client: client,
					req:    tsReq,
					url:    tsUrl,
					dur:    v.Duration,
				}
				tsDownloaders[i].Start()
			}
			ts := time.Now().UnixMilli()
			for i, v := range tsDownloaders {
				p.Debug("start download ts", "tsUrl", v.url.String())
				v.wg.Wait()
				if v.res != nil {
					info.TSCount++
					var reader io.Reader = v.res.Body
					closer := v.res.Body
					if p.SaveContext != nil && p.SaveContext.Err() == nil {
						savePath := p.SaveContext.Value("path").(string)
						os.MkdirAll(filepath.Join(savePath, p.PullJob.StreamPath), 0766)
						if f, err := os.OpenFile(filepath.Join(savePath, p.PullJob.StreamPath, filepath.Base(v.url.Path)), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666); err == nil {
							reader = io.TeeReader(v.res.Body, f)
							closer = f
						}
					}
					var tsBytes *util.Buffer
					switch p.PullJob.PublishConfig.RelayMode {
					case config.RelayModeRelay:
						tsBytes = &util.Buffer{}
						io.Copy(tsBytes, reader)
					case config.RelayModeMix:
						tsBytes = &util.Buffer{}
						reader = io.TeeReader(reader, tsBytes)
						fallthrough
					case config.RelayModeRemux:
						tsReader.Feed(reader)
					}
					if tsBytes != nil {
						tsFilename := fmt.Sprintf("%d_%d.ts", ts, i)
						tsFilePath := p.PullJob.StreamPath + "/" + tsFilename
						ss := strings.Split(p.PullJob.StreamPath, "/")
						var plInfo = PlaylistInf{
							URL:      fmt.Sprintf("%s/%s", ss[len(ss)-1], tsFilename),
							Duration: v.dur,
							FilePath: tsFilePath,
						}
						relayPlayList.WriteInf(plInfo)
						p.memoryTs.Store(tsFilePath, *tsBytes)
						next := tsRing.Next()
						if next.Value != "" {
							item, _ := p.memoryTs.LoadAndDelete(next.Value)
							if item == nil {
								p.Warn("memoryTs delete nil", "tsFilePath", next.Value)
							} else {
								// item.Recycle()
							}
						}
						next.Value = tsFilePath
						tsRing = next
					}
					closer.Close()
				} else if v.err != nil {
					p.Error("reqTs", "streamPath", p.PullJob.StreamPath, "err", v.err)
				} else {
					p.Error("reqTs", "streamPath", p.PullJob.StreamPath)
				}
				p.Debug("finish download ts", "tsUrl", v.url.String())
			}
			if p.PullJob.PublishConfig.RelayMode != config.RelayModeRemux {
				m3u8 := string(plBuffer)
				p.Debug("write m3u8", "streamPath", p.PullJob.StreamPath, "m3u8", m3u8)
				MemoryM3u8.Store(p.PullJob.StreamPath, m3u8)
			}
		} else {
			p.Error("readM3u8", "streamPath", p.PullJob.StreamPath, "err", err2)
			errcount++
			if errcount > 10 {
				return err2
			}
		}
	}
	return
}
