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
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

type Puller struct {
	task.Job
	PullJob     m7s.PullJob
	Video       M3u8Info
	Audio       M3u8Info
	TsHead      http.Header     `json:"-" yaml:"-"` //用于提供cookie等特殊身份的http头
	SaveContext context.Context `json:"-" yaml:"-"` //用来保存ts文件到服务器
	memoryTs    sync.Map
}

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "http") || strings.HasSuffix(conf.URL, ".m3u8") {
		p := &Puller{}
		p.SetDescription(task.OwnerTypeKey, "HLSPuller")
		return p
	}
	if conf.Args.Get(util.StartKey) != "" {
		p := &RecordReader{}
		p.Type = "hls"
		p.SetDescription(task.OwnerTypeKey, "HLSRecordReader")
		return p
	}
	return nil
}

func (p *Puller) GetPullJob() *m7s.PullJob {
	return &p.PullJob
}

func (p *Puller) GetTs(key string) (any, bool) {
	return p.memoryTs.Load(key)
}

func (p *Puller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}
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
	return p.pull(&p.Video)
}

func (p *Puller) writePublisher(t *mpegts.MpegTsStream) {
	var audioCodec codec.FourCC
	var audioStreamType, videoStreamType byte
	for pes := range t.PESChan {
		if p.Err() != nil {
			continue
		}
		if pes.Header.Dts == 0 {
			pes.Header.Dts = pes.Header.Pts
		}
		switch pes.Header.StreamID & 0xF0 {
		case mpegts.STREAM_ID_VIDEO:
			if videoStreamType == 0 {
				for _, s := range t.PMT.Stream {
					videoStreamType = s.StreamType
					break
				}
			}
			switch videoStreamType {
			case mpegts.STREAM_TYPE_H264:
				var annexb pkg.AnnexB
				annexb.PTS = time.Duration(pes.Header.Pts)
				annexb.DTS = time.Duration(pes.Header.Dts)
				annexb.AppendOne(pes.Payload)
				p.PullJob.Publisher.WriteVideo(&annexb)
			case mpegts.STREAM_TYPE_H265:
				var annexb pkg.AnnexB
				annexb.PTS = time.Duration(pes.Header.Pts)
				annexb.DTS = time.Duration(pes.Header.Dts)
				annexb.Hevc = true
				annexb.AppendOne(pes.Payload)
				p.PullJob.Publisher.WriteVideo(&annexb)
			default:
				if audioStreamType == 0 {
					for _, s := range t.PMT.Stream {
						audioStreamType = s.StreamType
						switch s.StreamType {
						case mpegts.STREAM_TYPE_AAC:
							audioCodec = codec.FourCC_MP4A
						case mpegts.STREAM_TYPE_G711A:
							audioCodec = codec.FourCC_ALAW
						case mpegts.STREAM_TYPE_G711U:
							audioCodec = codec.FourCC_ULAW
						}
					}
				}
				switch audioStreamType {
				case mpegts.STREAM_TYPE_AAC:
					var adts pkg.ADTS
					adts.DTS = time.Duration(pes.Header.Dts)
					adts.AppendOne(pes.Payload)
					p.PullJob.Publisher.WriteAudio(&adts)
				default:
					var raw pkg.RawAudio
					raw.FourCC = audioCodec
					raw.Timestamp = time.Duration(pes.Header.Pts) * time.Millisecond / 90
					raw.AppendOne(pes.Payload)
					p.PullJob.Publisher.WriteAudio(&raw)
				}
			}
		}
	}
}

func (p *Puller) pull(info *M3u8Info) (err error) {
	//请求失败自动退出
	req := info.Req.WithContext(p.Context)
	client := p.PullJob.HTTPClient
	sequence := -1
	lastTs := make(map[string]bool)
	tsbuffer := make(chan io.ReadCloser)
	tsRing := util.NewRing[string](6)
	var tsReader *mpegts.MpegTsStream
	var closer io.Closer
	p.OnDispose(func() {
		if closer != nil {
			closer.Close()
		}
	})
	if p.PullJob.PublishConfig.RelayMode != config.RelayModeRelay {
		tsReader = &mpegts.MpegTsStream{
			PESChan:   make(chan *mpegts.MpegTsPESPacket, 50),
			PESBuffer: make(map[uint16]*mpegts.MpegTsPESPacket),
		}
		go p.writePublisher(tsReader)
		defer close(tsReader.PESChan)
	}
	defer close(tsbuffer)
	var maxResolution *m3u8.PlaylistItem
	for errcount := 0; err == nil; err = p.Err() {
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
					closer = v.res.Body
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
							Title:    fmt.Sprintf("%s/%s", ss[len(ss)-1], tsFilename),
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
