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

func (ps *PubSubBase) Init(p *Plugin, streamPath string) {
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
			ah = reflect.ValueOf(audioHandler)
		}
	}
	if videoHandler != nil {
		v1 := reflect.TypeOf(videoHandler).In(0)
		vt := s.Publisher.GetVideoTrack(v1)
		if vt != nil {
			vr = NewAVRingReader(vt)
			vh = reflect.ValueOf(videoHandler)
		}
	}
	var initState = 0

	var subMode = s.SubMode //订阅模式
	if s.Args.Has(s.SubModeArgName) {
		subMode, _ = strconv.Atoi(s.Args.Get(s.SubModeArgName))
	}
	var audioFrame, videoFrame *AVFrame
	for err := s.Err(); err == nil; err = s.Err() {
		if vr != nil {
			for err == nil {
				err = vr.ReadFrame(subMode)
				if err == nil {
					err = s.Err()
				}
				if err != nil {
					s.Stop(err)
					// stopReason = zap.Error(err)
					return
				}
				videoFrame = &vr.Value
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
							ah.Call([]reflect.Value{reflect.ValueOf(audioFrame.Wrap)})
							audioFrame = nil
							break
						}
					} else if initState++; initState >= 2 {
						break
					}
				}

				if !s.IFrameOnly || videoFrame.Wrap.IsIDR() {
					vh.Call([]reflect.Value{reflect.ValueOf(videoFrame.Wrap)})
				} else {
					// fmt.Println("skip video", frame.Sequence)
				}
			}
		}
	}
}
