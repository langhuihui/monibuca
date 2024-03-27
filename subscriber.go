package m7s

import (
	"context"
	"log/slog"
	"net/url"
	"reflect"
	"strconv"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type PubSubBase struct {
	ID                      string
	*slog.Logger            `json:"-" yaml:"-"`
	context.Context         `json:"-" yaml:"-"`
	context.CancelCauseFunc `json:"-" yaml:"-"`
	Plugin                  *Plugin
	StartTime               time.Time
	StreamPath              string
	Args                    url.Values
}

func (ps *PubSubBase) Stop(err error) {
	ps.Error(err.Error())
	ps.CancelCauseFunc(err)
}

func (ps *PubSubBase) Init(ctx context.Context, p *Plugin, streamPath string) {
	ps.Plugin = p
	ps.Context, ps.CancelCauseFunc = context.WithCancelCause(p.Context)
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
	}
	ps.Logger = p.With("streamPath", ps.StreamPath)
	ps.StartTime = time.Now()
}

type Subscriber struct {
	PubSubBase
	config.Subscribe
	Publisher *Publisher
}

type ISubscriberHandler[T IAVFrame] func(data T)

func (s *Subscriber) Handle(audioHandler, videoHandler any) {
	var ar, vr *AVRingReader
	var ah, vh reflect.Value
	if audioHandler != nil {
		a1 := reflect.TypeOf(audioHandler).In(0)
		at := s.Publisher.GetAudioTrack(a1)
		if at != nil {
			ar = NewAVRingReader(at)
			ar.Logger = s.Logger.With("reader", a1.Name())
			ah = reflect.ValueOf(audioHandler)
		}
	}
	if videoHandler != nil {
		v1 := reflect.TypeOf(videoHandler).In(0)
		vt := s.Publisher.GetVideoTrack(v1)
		if vt != nil {
			vr = NewAVRingReader(vt)
			vr.Logger = s.Logger.With("reader", v1.Name())
			vh = reflect.ValueOf(videoHandler)
		}
	}
	var initState = 0

	var subMode = s.SubMode //订阅模式
	if s.Args.Has(s.SubModeArgName) {
		subMode, _ = strconv.Atoi(s.Args.Get(s.SubModeArgName))
	}
	var audioFrame, videoFrame, lastSentAF, lastSentVF *AVFrame

	defer func() {
		if lastSentVF != nil {
			lastSentVF.ReaderLeave()
		}
		if lastSentAF != nil {
			lastSentAF.ReaderLeave()
		}
		s.Info("subscriber stopped", "reason", context.Cause(s.Context))
	}()
	sendAudioFrame := func() {
		lastSentAF = audioFrame
		s.Debug("send audio frame", "frame", audioFrame.Sequence)
		ah.Call([]reflect.Value{reflect.ValueOf(audioFrame.Wrap)})
	}
	sendVideoFrame := func() {
		lastSentVF = videoFrame
		s.Debug("send video frame", "frame", videoFrame.Sequence)
		vh.Call([]reflect.Value{reflect.ValueOf(videoFrame.Wrap)})
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
					s.Debug("video codec changed")
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
		}
	}
}
