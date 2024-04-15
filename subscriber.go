package m7s

import (
	"context"
	"io"
	"net/url"
	"reflect"
	"strconv"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type PubSubBase struct {
	Unit
	ID           int
	Plugin       *Plugin `json:"-" yaml:"-"`
	StreamPath   string
	Args         url.Values
	TimeoutTimer *time.Timer `json:"-" yaml:"-"`
	io.Closer    `json:"-" yaml:"-"`
}

func (p *PubSubBase) GetKey() int {
	return p.ID
}

func (ps *PubSubBase) Init(p *Plugin, streamPath string, options ...any) {
	ps.Plugin = p
	ctx := p.Context
	for _, option := range options {
		switch v := option.(type) {
		case context.Context:
			ctx = v
		case io.Closer:
			ps.Closer = v
		}
	}
	ps.Context, ps.CancelCauseFunc = context.WithCancelCause(ctx)
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
	}
	ps.StartTime = time.Now()
}

type Subscriber struct {
	PubSubBase
	config.Subscribe
	Publisher *Publisher
}

type SubscriberHandler struct {
	OnAudio any
	OnVideo any
}

func (s *Subscriber) Handle(handler SubscriberHandler) {
	var ar, vr *AVRingReader
	var ah, vh reflect.Value
	var a1, v1 reflect.Type
	var initState = 0
	var subMode = s.SubMode //订阅模式
	if s.Args.Has(s.SubModeArgName) {
		subMode, _ = strconv.Atoi(s.Args.Get(s.SubModeArgName))
	}
	var audioFrame, videoFrame, lastSentAF, lastSentVF *AVFrame
	if handler.OnAudio != nil && s.SubAudio {
		a1 = reflect.TypeOf(handler.OnAudio).In(0)
	}
	if handler.OnVideo != nil && s.SubVideo {
		v1 = reflect.TypeOf(handler.OnVideo).In(0)
	}
	createAudioReader := func() {
		if s.Publisher == nil || a1 == nil {
			return
		}
		if at := s.Publisher.GetAudioTrack(a1); at != nil {
			ar = NewAVRingReader(at)
			ar.Logger = s.Logger.With("reader", a1.String())
			ar.Info("start read")
			ah = reflect.ValueOf(handler.OnAudio)
		}
	}
	createVideoReader := func() {
		if s.Publisher == nil || v1 == nil {
			return
		}
		if vt := s.Publisher.GetVideoTrack(v1); vt != nil {
			vr = NewAVRingReader(vt)
			vr.Logger = s.Logger.With("reader", v1.String())
			vr.Info("start read")
			vh = reflect.ValueOf(handler.OnVideo)
		}
	}
	createAudioReader()
	createVideoReader()
	defer func() {
		if lastSentVF != nil {
			lastSentVF.ReaderLeave()
		}
		if lastSentAF != nil {
			lastSentAF.ReaderLeave()
		}
	}()
	sendAudioFrame := func() {
		lastSentAF = audioFrame
		s.Debug("send audio frame", "seq", audioFrame.Sequence)
		res := ah.Call([]reflect.Value{reflect.ValueOf(audioFrame.Wrap)})
		if len(res) > 0 && !res[0].IsNil() {
			s.Stop(res[0].Interface().(error))
		}
	}
	sendVideoFrame := func() {
		lastSentVF = videoFrame
		s.Debug("send video frame", "seq", videoFrame.Sequence, "data", videoFrame.Wrap.Print(), "size", videoFrame.Wrap.GetSize())
		res := vh.Call([]reflect.Value{reflect.ValueOf(videoFrame.Wrap)})
		if len(res) > 0 && !res[0].IsNil() {
			s.Stop(res[0].Interface().(error))
		}
	}
	for err := s.Err(); err == nil; err = s.Err() {
		if vr != nil {
			for err == nil {
				err = vr.ReadFrame(subMode)
				if err == nil {
					videoFrame = &vr.Value
					err = s.Err()
				} else {
					s.Stop(err)
				}
				if err != nil {
					return
				}
				// fmt.Println("video", s.VideoReader.Track.PreFrame().Sequence-frame.Sequence)
				if videoFrame.Wrap.IsIDR() && vr.DecConfChanged() {
					vr.LastCodecCtx = vr.Track.ICodecCtx
					s.Debug("video codec changed", "data", vr.Track.ICodecCtx.GetSequenceFrame().Print())
					vh.Call([]reflect.Value{reflect.ValueOf(vr.Track.ICodecCtx.GetSequenceFrame())})
				}
				if ar != nil {
					if audioFrame != nil {
						if util.Conditoinal(s.SyncMode == 0, videoFrame.Timestamp > audioFrame.Timestamp, videoFrame.WriteTime.After(audioFrame.WriteTime)) {
							// fmt.Println("switch audio", audioFrame.CanRead)
							sendAudioFrame()
							audioFrame = nil
							break
						}
					} else if initState++; initState >= 2 {
						break
					}
				}

				if !s.IFrameOnly || videoFrame.Wrap.IsIDR() {
					sendVideoFrame()
				}
			}
		} else {
			createVideoReader()
		}
		// 正常模式下或者纯音频模式下，音频开始播放
		if ar != nil {
			for err == nil {
				switch ar.State {
				case READSTATE_INIT:
					if vr != nil {
						ar.FirstTs = vr.FirstTs

					}
				case READSTATE_NORMAL:
					if vr != nil {
						ar.SkipTs = vr.SkipTs
					}
				}
				err = ar.ReadFrame(subMode)
				if err == nil {
					audioFrame = &ar.Value
					err = s.Err()
				} else {
					s.Stop(err)
				}
				if err != nil {
					return
				}
				// fmt.Println("audio", s.AudioReader.Track.PreFrame().Sequence-frame.Sequence)
				if ar.DecConfChanged() {
					ar.LastCodecCtx = ar.Track.ICodecCtx
					if sf := ar.Track.ICodecCtx.GetSequenceFrame(); sf != nil {
						ah.Call([]reflect.Value{reflect.ValueOf(sf)})
					}
				}
				if vr != nil && videoFrame != nil {
					if util.Conditoinal(s.SyncMode == 0, audioFrame.Timestamp > videoFrame.Timestamp, audioFrame.WriteTime.After(videoFrame.WriteTime)) {
						sendVideoFrame()
						videoFrame = nil
						break
					}
				}
				if audioFrame.Timestamp >= ar.SkipTs {
					sendAudioFrame()
				} else {
					s.Debug("skip audio", "frame.AbsTime", audioFrame.Timestamp, "s.AudioReader.SkipTs", ar.SkipTs)
				}
			}
		} else {
			createAudioReader()
		}
	}
}
