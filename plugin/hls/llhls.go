package plugin_hls

import (
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"golang.org/x/exp/slices"
	"m7s.live/v5"
	. "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	"m7s.live/v5/pkg/util"
)

var _ = InstallPlugin[LLHLSPlugin](m7s.PluginMeta{
	NewTransformer: NewLLHLSTransform,
})
var llwriting util.Collection[string, *LLMuxer]

func init() {
	llwriting.L = &sync.RWMutex{}
}

func NewLLHLSTransform() ITransformer {
	ret := &LLMuxer{}
	return ret
}

type LLHLSPlugin struct {
	Plugin
}

func (c *LLHLSPlugin) Start() (err error) {
	_, port, _ := strings.Cut(c.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		c.PlayAddr = append(c.PlayAddr, "http://{hostName}/llhls/{streamPath}/index.m3u8")
	} else if port != "" {
		c.PlayAddr = append(c.PlayAddr, fmt.Sprintf("http://{hostName}:%s/llhls/{streamPath}/index.m3u8", port))
	}
	_, port, _ = strings.Cut(c.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		c.PlayAddr = append(c.PlayAddr, "https://{hostName}/llhls/{streamPath}/index.m3u8")
	} else if port != "" {
		c.PlayAddr = append(c.PlayAddr, fmt.Sprintf("https://{hostName}:%s/llhls/{streamPath}/index.m3u8", port))
	}
	return
}

func (c *LLHLSPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, ".html") {
		w.Write([]byte(`<html><body><video src="/llhls/` + strings.TrimSuffix(r.URL.Path, ".html") + `/index.m3u8"></video></body></html>`))
		return
	}
	streamPath := strings.TrimPrefix(r.URL.Path, "/")
	streamPath = path.Dir(streamPath)
	if llwriting.Has(streamPath) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/"+streamPath)
		writer, ok := llwriting.Get(streamPath)
		if ok {
			writer.Handle(w, r)
		}
		return
	} else {
		w.Write([]byte(`<html><body><video src="/llhls/` + streamPath + `/index.m3u8"></video></body></html>`))
	}
}

type LLMuxer struct {
	DefaultTransformer
	*gohlslib.Muxer
}

func (ll *LLMuxer) GetKey() string {
	return ll.TransformJob.StreamPath
}

func (ll *LLMuxer) Start() (err error) {
	return ll.TransformJob.Subscribe()
}

func (ll *LLMuxer) Run() (err error) {
	llwriting.Set(ll)
	subscriber := ll.TransformJob.Subscriber
	ll.Muxer = &gohlslib.Muxer{
		Variant:            gohlslib.MuxerVariantLowLatency,
		SegmentCount:       7,
		SegmentMinDuration: 1 * time.Second,
	}

	if conf, ok := ll.TransformJob.Config.Input.(string); ok {
		ss := strings.Split(conf, "x")
		if len(ss) != 2 {
			return fmt.Errorf("invalid input config %s", conf)
		}
		ll.Muxer.SegmentMinDuration, err = time.ParseDuration(strings.TrimSpace(ss[0]))
		if err != nil {
			return
		}
		ll.Muxer.SegmentCount, err = strconv.Atoi(strings.TrimSpace(ss[1]))
		if err != nil {
			return
		}
	}

	var videoFunc = func(v *pkg.AVFrame) (err error) {
		return nil
	}
	if ctx := subscriber.Publisher.GetVideoCodecCtx(); ctx != nil {
		ll.Muxer.VideoTrack = &gohlslib.Track{}
		switch ctx := ctx.GetBase().(type) {
		case *codec.H264Ctx:
			ll.Muxer.VideoTrack.Codec = &codecs.H264{
				SPS: ctx.SPS(),
				PPS: ctx.PPS(),
			}
			videoFunc = func(v *pkg.AVFrame) (err error) {
				ts := v.Timestamp
				var au [][]byte
				if subscriber.VideoReader.Value.IDR {
					au = append(au, ctx.SPS(), ctx.PPS())
				}
				for buffer := range v.Raw.(*pkg.Nalus).RangePoint {
					au = append(au, buffer.Buffers...)
				}
				return ll.Muxer.WriteH264(time.Now().Add(ts-ll.Muxer.SegmentMinDuration), v.GetPTS(), au)
			}
		case *codec.H265Ctx:
			ll.Muxer.VideoTrack.Codec = &codecs.H265{
				SPS: ctx.SPS(),
				PPS: ctx.PPS(),
				VPS: ctx.VPS(),
			}
			videoFunc = func(v *pkg.AVFrame) (err error) {
				var au [][]byte
				if subscriber.VideoReader.Value.IDR {
					au = append(au, ctx.VPS(), ctx.SPS(), ctx.PPS())
				}
				for buffer := range v.Raw.(*pkg.Nalus).RangePoint {
					au = append(au, buffer.Buffers...)
				}
				return ll.Muxer.WriteH265(time.Now().Add(v.Timestamp-ll.Muxer.SegmentMinDuration), v.GetPTS(), au)
			}
		}
	}
	if ctx := subscriber.Publisher.GetAudioCodecCtx(); ctx != nil {
		ll.Muxer.AudioTrack = &gohlslib.Track{}
		switch ctx := ctx.GetBase().(type) {
		case *codec.AACCtx:
			var config mpeg4audio.Config
			config.Unmarshal(ctx.ConfigBytes)
			ll.Muxer.AudioTrack.Codec = &codecs.MPEG4Audio{
				Config: config,
			}
		}
	}

	err = ll.Muxer.Start()
	if err != nil {
		return
	}

	return PlayBlock(ll.TransformJob.Subscriber, func(audio *format.RawAudio) (err error) {
		now := time.Now()
		ts := audio.Timestamp
		return ll.Muxer.WriteMPEG4Audio(now.Add(ts-ll.Muxer.SegmentMinDuration), audio.GetDTS(), slices.Clone(audio.Buffers))
	}, videoFunc)
}

func (ll *LLMuxer) Dispose() {
	ll.Muxer.Close()
	llwriting.Remove(ll)
}
