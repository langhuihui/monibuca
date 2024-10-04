package m7s

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"m7s.live/m7s/v5/pkg/task"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var AVFrameType = reflect.TypeOf((*AVFrame)(nil))
var Owner task.TaskContextKey = "owner"

type PubSubBase struct {
	task.Job
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
		config.Parse(conf, cc)
	}
}

type SubscriberCollection = util.Collection[uint32, *Subscriber]

type Subscriber struct {
	PubSubBase
	config.Subscribe
	Publisher                  *Publisher
	waitPublishDone            *util.Promise
	AudioReader, VideoReader   *AVRingReader
	StartAudioTS, StartVideoTS time.Duration
}

func createSubscriber(p *Plugin, streamPath string, conf config.Subscribe) *Subscriber {
	subscriber := &Subscriber{Subscribe: conf, waitPublishDone: util.NewPromise(p)}
	subscriber.ID = task.GetNextTaskID()
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
		return publisher.WaitTrack()
	} else {
		for reg, streamPath := range server.StreamAlias {
			if g := reg.FindStringSubmatch(s.StreamPath); len(g) > 0 {
				for i, gg := range g {
					streamPath = strings.ReplaceAll(streamPath, fmt.Sprintf("$%d", i), gg)
				}
				if publisher, ok = server.Streams.Get(streamPath); ok {
					s.Description["alias"] = streamPath
					publisher.AddSubscriber(s)
					return publisher.WaitTrack()
				}
			}
		}
		if waitStream, ok := server.Waiting.Get(s.StreamPath); ok {
			waitStream.Add(s)
		} else {
			server.createWait(s.StreamPath).Add(s)
		}
		for plugin := range server.Plugins.Range {
			plugin.OnSubscribe(s)
		}
	}
	return
}

func (s *Subscriber) Dispose() {
	s.Plugin.Server.Subscribers.Remove(s)
	s.Info("unsubscribe", "reason", s.StopReason())
	if s.Publisher != nil {
		s.Publisher.RemoveSubscriber(s)
	} else if waitStream, ok := s.Plugin.Server.Waiting.Get(s.StreamPath); ok {
		waitStream.Remove(s)
	}
}

type PlayController struct {
	task.Task
	conn       net.Conn
	Subscriber *Subscriber
}

func (pc *PlayController) Go() (err error) {
	for err == nil {
		var b []byte
		b, err = wsutil.ReadClientBinary(pc.conn)
		if pc.Subscriber.Publisher == nil {
			continue
		}
		if len(b) >= 3 && [3]byte(b[:3]) == [3]byte{'c', 'm', 'd'} {
			switch b[3] {
			case 1: // pause
				pc.Subscriber.Publisher.Pause()
			case 2: // resume
				pc.Subscriber.Publisher.Resume()
			case 3: // seek
				pc.Subscriber.Publisher.Seek(time.Duration(binary.BigEndian.Uint32(b[4:8])))
			case 4: // speed
				pc.Subscriber.Publisher.Speed = float64(binary.BigEndian.Uint32(b[4:8])) / 100
			}
		}
	}
	return
}

func (s *Subscriber) CheckWebSocket(w http.ResponseWriter, r *http.Request) (conn net.Conn, err error) {
	if r.Header.Get("Upgrade") == "websocket" {
		conn, _, _, err = ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		var playController = &PlayController{
			Subscriber: s,
			conn:       conn,
		}
		s.AddTask(playController)
	}
	return
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
	task.Task
	s                          *Subscriber
	OnAudio                    func(A) error
	OnVideo                    func(V) error
	ProcessAudio, ProcessVideo chan func(*AVFrame)
}

func CreatePlayTask[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) task.ITask {
	return &SubscribeHandler[A, V]{
		s:       s,
		OnAudio: onAudio,
		OnVideo: onVideo,
	}
}

func PlayBlock[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) (err error) {
	handler := &SubscribeHandler[A, V]{
		s:       s,
		OnAudio: onAudio,
		OnVideo: onVideo,
	}
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
				if s.Enabled(s, task.TraceLevel) {
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
				if s.Enabled(s, task.TraceLevel) {
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
			if prePublisher != nil {
				if s.Publisher == nil {
					s.Info("publisher gone", "prePublisher", prePublisher.ID)
				} else {
					s.Info("publisher changed", "prePublisher", prePublisher.ID, "publisher", s.Publisher.ID)
				}
			} else {
				s.Info("publisher recover", "publisher", s.Publisher.ID)
			}
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
				} else if errors.Is(err, ErrDiscard) {
					s.VideoReader = nil
					break
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
						if util.Conditional(s.SyncMode == 0, videoFrame.Timestamp > audioFrame.Timestamp, videoFrame.WriteTime.After(audioFrame.WriteTime)) {
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
				//switch ar.State {
				//case READSTATE_INIT:
				//	if vr != nil {
				//		ar.FirstTs = vr.FirstTs
				//
				//	}
				//case READSTATE_NORMAL:
				//	if vr != nil {
				//		ar.SkipTs = vr.SkipTs
				//	}
				//}
				if err = ar.ReadFrame(&s.Subscribe); err == nil {
					if prePublisher != s.Publisher {
						break
					}
					audioFrame = &ar.Value
					err = s.Err()
				} else if errors.Is(err, ErrDiscard) {
					s.AudioReader = nil
					break
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
					if util.Conditional(s.SyncMode == 0, audioFrame.Timestamp > videoFrame.Timestamp, audioFrame.WriteTime.After(videoFrame.WriteTime)) {
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
