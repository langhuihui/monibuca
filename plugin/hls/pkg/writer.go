package hls

import (
	"container/ring"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/util"
)

func NewTransform() m7s.ITransformer {
	ret := &HLSWriter{
		Window:   3,
		Fragment: 5 * time.Second,
	}
	return ret
}

type HLSWriter struct {
	m7s.DefaultTransformer
	Window             int
	Fragment           time.Duration
	M3u8               util.Buffer
	ts                 *TsInMemory
	write_time         time.Duration
	memoryTs           sync.Map
	hls_segment_count  uint32 // hls segment count
	playlist           Playlist
	infoRing           *ring.Ring
	hls_playlist_count uint32
	hls_segment_window uint32
	lastReadTime       time.Time
}

func (w *HLSWriter) Start() (err error) {
	return w.TransformJob.Subscribe()
}

func (w *HLSWriter) GetTs(key string) (any, bool) {
	w.lastReadTime = time.Now()
	return w.memoryTs.Load(key)
}

func (w *HLSWriter) Run() (err error) {
	if conf, ok := w.TransformJob.Config.Input.(string); ok {
		ss := strings.Split(conf, "x")
		if len(ss) != 2 {
			return fmt.Errorf("invalid input config %s", conf)
		}
		w.Fragment, err = time.ParseDuration(strings.TrimSpace(ss[0]))
		if err != nil {
			return
		}
		w.Window, err = strconv.Atoi(strings.TrimSpace(ss[1]))
		if err != nil {
			return
		}
	}
	subscriber := w.TransformJob.Subscriber
	w.hls_segment_window = uint32(w.Window) + 1
	w.infoRing = ring.New(w.Window + 1)
	w.playlist = Playlist{
		Writer:         &w.M3u8,
		Version:        3,
		Sequence:       0,
		Targetduration: int(w.Fragment / time.Millisecond / 666), // hlsFragment * 1.5 / 1000
	}
	MemoryTs.Store(w.TransformJob.StreamPath, w)
	var audioCodec, videoCodec codec.FourCC
	if subscriber.Publisher.HasAudioTrack() {
		audioCodec = subscriber.Publisher.AudioTrack.FourCC()
	}
	if subscriber.Publisher.HasVideoTrack() {
		videoCodec = subscriber.Publisher.VideoTrack.FourCC()
	}
	w.ts = &TsInMemory{}
	pesAudio, pesVideo := mpegts.CreatePESWriters()
	w.ts.WritePMTPacket(audioCodec, videoCodec)
	return m7s.PlayBlock(subscriber, func(audio *format.Mpeg2Audio) error {
		pesAudio.Pts = uint64(subscriber.AudioReader.AbsTime) * 90
		return pesAudio.WritePESPacket(audio.Memory, &w.ts.RecyclableMemory)
	}, func(video *mpegts.VideoFrame) (err error) {
		vr := w.TransformJob.Subscriber.VideoReader
		if vr.Value.IDR {
			if err = w.checkFragment(video.Timestamp); err != nil {
				return
			}
		}
		pesVideo.IsKeyFrame = video.IDR
		pesVideo.Pts = uint64(vr.AbsTime+video.GetCTS32()) * 90
		pesVideo.Dts = uint64(vr.AbsTime) * 90
		return pesVideo.WritePESPacket(video.Memory, &w.ts.RecyclableMemory)
	})
}

func (w *HLSWriter) checkFragment(ts time.Duration) (err error) {
	// 当前的时间戳减去上一个ts切片的时间戳
	if dur := ts - w.write_time; dur >= w.Fragment {
		streamPath := w.TransformJob.StreamPath
		ss := strings.Split(streamPath, "/")
		// fmt.Println("time :", video.Timestamp, tsSegmentTimestamp)
		if dur == ts && w.write_time == 0 { //时间戳不对的情况，首个默认为2s
			dur = time.Duration(2) * time.Second
		}
		num := w.hls_segment_count
		tsFilename := strconv.FormatInt(time.Now().Unix(), 10) + "_" + strconv.FormatUint(uint64(num), 10) + ".ts"
		tsFilePath := streamPath + "/" + tsFilename

		// println(hls.currentTs.Length)

		w.Debug("write ts", "tsFilePath", tsFilePath)
		w.memoryTs.Store(tsFilePath, w.ts)
		w.ts = &TsInMemory{
			PMT: w.ts.PMT,
		}
		if w.playlist.Targetduration < int(dur.Seconds()) {
			w.playlist.Targetduration = int(math.Ceil(dur.Seconds()))
		}
		if w.M3u8.Len() == 0 {
			w.playlist.Init()
		}
		inf := PlaylistInf{
			//浮点计算精度
			Duration: dur.Seconds(),
			URL:      fmt.Sprintf("%s/%s", ss[len(ss)-1], tsFilename),
			FilePath: tsFilePath,
		}

		if w.hls_segment_count > 0 {
			if w.hls_playlist_count >= uint32(w.Window) {
				w.M3u8.Reset()
				if err = w.playlist.Init(); err != nil {
					return
				}
				//playlist起点是ring.next，长度是len(ring)-1
				for p := w.infoRing.Next(); p != w.infoRing; p = p.Next() {
					w.playlist.WriteInf(p.Value.(PlaylistInf))
				}
			} else {
				if err = w.playlist.WriteInf(w.infoRing.Prev().Value.(PlaylistInf)); err != nil {
					return
				}
			}
			MemoryM3u8.Store(w.TransformJob.StreamPath, string(w.M3u8))
			w.hls_playlist_count++
		}

		if w.hls_segment_count >= w.hls_segment_window {
			if mts, loaded := w.memoryTs.LoadAndDelete(w.infoRing.Value.(PlaylistInf).FilePath); loaded {
				mts.(*TsInMemory).Recycle()
			}
			w.infoRing.Value = inf
			w.infoRing = w.infoRing.Next()
		} else {
			w.infoRing.Value = inf
			w.infoRing = w.infoRing.Next()
		}
		w.hls_segment_count++
		w.write_time = ts

	}
	return
}

func (w *HLSWriter) Dispose() {
	MemoryM3u8.Delete(w.TransformJob.StreamPath)
	if ts, loaded := MemoryTs.LoadAndDelete(w.TransformJob.StreamPath); loaded {
		ts.(*HLSWriter).memoryTs.Range(func(key, value any) bool {
			value.(*TsInMemory).Recycle()
			return true
		})
	}
}
