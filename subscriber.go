package m7s

import (
	"errors"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var AVFrameType = reflect.TypeOf((*AVFrame)(nil))

type PubSubBase struct {
	util.MarcoTask
	Plugin       *Plugin
	StreamPath   string
	Args         url.Values
	TimeoutTimer *time.Timer
}

func (ps *PubSubBase) Init(streamPath string, conf any) {
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
	}
	ps.Description = map[string]any{
		"streamPath": ps.StreamPath,
		"args":       ps.Args,
		"plugin":     ps.Plugin.Meta.Name,
	}
	// args to config
	if len(ps.Args) != 0 {
		var c config.Config
		c.Parse(conf)
		ignores, cc := make(map[string]struct{}), make(map[string]any)
		for key, value := range ps.Args {
			if strings.HasSuffix(key, "ArgName") {
				targetArgName := strings.TrimSuffix(key, "ArgName")
				cc[strings.ToLower(targetArgName)] = ps.Args.Get(value[0])[0]
				ignores[value[0]] = struct{}{}
				delete(cc, value[0])
			} else if _, ok := ignores[key]; !ok {
				cc[strings.ToLower(key)] = value[0]
			}
		}
		c.ParseModifyFile(cc)
	}
}

type SubscriberCollection = util.Collection[uint32, *Subscriber]

type Subscriber struct {
	PubSubBase
	config.Subscribe
	Publisher                  *Publisher
	AudioReader, VideoReader   *AVRingReader
	StartAudioTS, StartVideoTS time.Duration
}

func createSubscriber(p *Plugin, streamPath string, conf config.Subscribe) *Subscriber {
	subscriber := &Subscriber{Subscribe: conf}
	subscriber.ID = util.GetNextTaskID()
	subscriber.Plugin = p
	subscriber.TimeoutTimer = time.NewTimer(subscriber.WaitTimeout)
	subscriber.Logger = p.Logger.With("streamPath", streamPath, "sId", subscriber.ID)
	subscriber.Init(streamPath, &subscriber.Subscribe)
	if subscriber.Subscribe.BufferTime > 0 {
		subscriber.Subscribe.SubMode = SUBMODE_BUFFER
	}
	return subscriber
}

func (s *Subscriber) Start() (err error) {
	server := s.Plugin.Server
	server.Subscribers.Add(s)
	s.Info("subscribe")
	if publisher, ok := server.Streams.Get(s.StreamPath); ok {
		publisher.AddSubscriber(s)
		err = publisher.WaitTrack()
	} else if waitStream, ok := server.Waiting.Get(s.StreamPath); ok {
		waitStream.Add(s)
	} else {
		server.createWait(s.StreamPath).Add(s)
		for plugin := range server.Plugins.Range {
			if remoteURL := plugin.GetCommonConf().Pull.CheckPullOnSub(s.StreamPath); remoteURL != "" {
				if plugin.Meta.Puller != nil {
					plugin.handler.Pull(s.StreamPath, remoteURL)
				}
			}
		}
	}
	return
}

func (s *Subscriber) Dispose() {
	s.Plugin.Server.Subscribers.Remove(s)
	s.Info("unsubscribe", "reason", s.StopReason())
	if s.Publisher != nil {
		s.Publisher.RemoveSubscriber(s)
	}
}

func (s *Subscriber) createAudioReader(dataType reflect.Type, startAudioTs time.Duration) (awi int) {
	if s.Publisher == nil || dataType == nil {
		return
	}
	var at *AVTrack
	if dataType == AVFrameType {
		at = s.Publisher.AudioTrack.AVTrack
		awi = -1
	} else {
		at = s.Publisher.GetAudioTrack(dataType)
		if at != nil {
			awi = at.WrapIndex
		}
	}
	if at != nil {
		if err := at.WaitReady(); err != nil {
			return
		}
		ar := NewAVRingReader(at)
		s.AudioReader = ar
		ar.StartTs = startAudioTs
		ar.Logger = s.Logger.With("reader", dataType.String())
		ar.Info("start read")
	}
	return
}

func (s *Subscriber) createVideoReader(dataType reflect.Type, startVideoTs time.Duration) (vwi int) {
	if s.Publisher == nil || dataType == nil {
		return
	}
	var vt *AVTrack
	if dataType == AVFrameType {
		vt = s.Publisher.VideoTrack.AVTrack
		vwi = -1
	} else {
		vt = s.Publisher.GetVideoTrack(dataType)
		if vt != nil {
			vwi = vt.WrapIndex
		}
	}
	if vt != nil {
		if err := vt.WaitReady(); err != nil {
			return
		}
		vr := NewAVRingReader(vt)
		vr.StartTs = startVideoTs
		s.VideoReader = vr
		vr.Logger = s.Logger.With("reader", dataType.String())
		vr.Info("start read")
	}
	return
}

type SubscribeHandler[A any, V any] struct {
	util.Task
	s            *Subscriber
	OnAudio      func(A) error
	OnVideo      func(V) error
	ProcessAudio chan func(*AVFrame)
	ProcessVideo chan func(*AVFrame)
}

func CreatePlayTask[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) util.ITask {
	var handler SubscribeHandler[A, V]
	handler.s = s
	handler.OnAudio = onAudio
	handler.OnVideo = onVideo
	return &handler
}

func PlayBlock[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) (err error) {
	var handler SubscribeHandler[A, V]
	handler.s = s
	handler.OnAudio = onAudio
	handler.OnVideo = onVideo
	err = handler.Start()
	s.Stop(err)
	return
}

func (handler *SubscribeHandler[A, V]) Start() (err error) {
	var a1, v1 reflect.Type
	s := handler.s
	startAudioTs, startVideoTs := s.StartAudioTS, s.StartVideoTS
	var initState = 0
	prePublisher := s.Publisher
	var audioFrame, videoFrame *AVFrame
	if s.SubAudio {
		a1 = reflect.TypeOf(handler.OnAudio).In(0)
	}
	if s.SubVideo {
		v1 = reflect.TypeOf(handler.OnVideo).In(0)
	}
	awi := s.createAudioReader(a1, startAudioTs)
	vwi := s.createVideoReader(v1, startVideoTs)
	defer func() {
		if s.AudioReader != nil {
			s.AudioReader.StopRead()
		}
		if s.VideoReader != nil {
			s.VideoReader.StopRead()
		}
	}()
	sendAudioFrame := func() (err error) {
		if awi >= 0 {
			if len(audioFrame.Wraps) > awi {
				if s.Enabled(s, util.TraceLevel) {
					s.Trace("send audio frame", "seq", audioFrame.Sequence)
				}
				err = handler.OnAudio(audioFrame.Wraps[awi].(A))
			} else {
				s.AudioReader.StopRead()
			}
		} else {
			err = handler.OnAudio(any(audioFrame).(A))
		}
		if err != nil && !errors.Is(err, ErrInterrupt) {
			s.Stop(err)
		}
		if handler.ProcessAudio != nil {
			if f, ok := <-handler.ProcessAudio; ok {
				f(audioFrame)
			}
		}
		audioFrame = nil
		return
	}
	sendVideoFrame := func() (err error) {
		if vwi >= 0 {
			if len(videoFrame.Wraps) > vwi {
				if s.Enabled(s, util.TraceLevel) {
					s.Trace("send video frame", "seq", videoFrame.Sequence, "data", videoFrame.Wraps[vwi].String(), "size", videoFrame.Wraps[vwi].GetSize())
				}
				err = handler.OnVideo(videoFrame.Wraps[vwi].(V))
			} else {
				s.VideoReader.StopRead()
			}
		} else {
			err = handler.OnVideo(any(videoFrame).(V))
		}
		if err != nil && !errors.Is(err, ErrInterrupt) {
			s.Stop(err)
		}
		if handler.ProcessVideo != nil {
			if f, ok := <-handler.ProcessVideo; ok {
				f(videoFrame)
			}
		}
		videoFrame = nil
		return
	}
	checkPublisherChange := func() {
		if prePublisher != s.Publisher {
			s.Info("publisher changed", "prePublisher", prePublisher.ID, "publisher", s.Publisher.ID)
			if s.AudioReader != nil {
				startAudioTs = time.Duration(s.AudioReader.AbsTime) * time.Millisecond
				s.AudioReader.StopRead()
				s.AudioReader = nil
			}
			if s.VideoReader != nil {
				startVideoTs = time.Duration(s.VideoReader.AbsTime) * time.Millisecond
				s.VideoReader.StopRead()
				s.VideoReader = nil
			}
			awi = s.createAudioReader(a1, startAudioTs)
			vwi = s.createVideoReader(v1, startVideoTs)
			prePublisher = s.Publisher
		}
	}
	for err == nil {
		err = s.Err()
		ar, vr := s.AudioReader, s.VideoReader
		if vr != nil {
			for err == nil {
				err = vr.ReadFrame(&s.Subscribe)
				if prePublisher != s.Publisher {
					break
				}
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
				if videoFrame.IDR && vr.DecConfChanged() {
					vr.LastCodecCtx = vr.Track.ICodecCtx
					if seqFrame := vr.Track.SequenceFrame; seqFrame != nil {
						s.Debug("video codec changed", "data", seqFrame.String())
						err = handler.OnVideo(seqFrame.(V))
					}
				}
				if ar != nil {
					if audioFrame != nil {
						if util.Conditoinal(s.SyncMode == 0, videoFrame.Timestamp > audioFrame.Timestamp, videoFrame.WriteTime.After(audioFrame.WriteTime)) {
							// fmt.Println("switch audio", audioFrame.CanRead)
							err = sendAudioFrame()
							break
						}
					} else if initState++; initState >= 2 {
						break
					}
				}

				if !s.IFrameOnly || videoFrame.IDR {
					err = sendVideoFrame()
				}
				if ar == nil {
					break
				}
			}
		} else {
			vwi = s.createVideoReader(v1, startVideoTs)
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
				if err = ar.ReadFrame(&s.Subscribe); err == nil {
					if prePublisher != s.Publisher {
						break
					}
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
					if seqFrame := ar.Track.SequenceFrame; seqFrame != nil {
						err = handler.OnAudio(seqFrame.(A))
					}
				}
				if vr != nil && videoFrame != nil {
					if util.Conditoinal(s.SyncMode == 0, audioFrame.Timestamp > videoFrame.Timestamp, audioFrame.WriteTime.After(videoFrame.WriteTime)) {
						err = sendVideoFrame()
						break
					}
				}
				if audioFrame.Timestamp >= ar.SkipTs {
					err = sendAudioFrame()
				} else {
					s.Debug("skip audio", "frame.AbsTime", audioFrame.Timestamp, "s.AudioReader.SkipTs", ar.SkipTs)
				}
			}
		} else {
			awi = s.createAudioReader(a1, startAudioTs)
		}
		checkPublisherChange()
		runtime.Gosched()
	}
	return
}
