package m7s

import (
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"m7s.live/v5/pkg/task"

	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
)

var AVFrameType = reflect.TypeOf((*AVFrame)(nil))
var Owner task.TaskContextKey = "owner"

const (
	SubscribeTypePush      = "push"
	SubscribeTypeServer    = "server"
	SubscribeTypeVod       = "vod"
	SubscribeTypeTransform = "transform"
	SubscribeTypeReplay    = "replay"
	SubscribeTypeAPI       = "api"
)

type PubSubBase struct {
	task.Job
	Plugin     *Plugin
	Type       string
	StreamPath string
	Args       url.Values
	RemoteAddr string
}

func (ps *PubSubBase) Init(streamPath string, conf any) {
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
	}
	ps.SetDescriptions(task.Description{
		"streamPath": ps.StreamPath,
		"args":       ps.Args,
		"plugin":     ps.Plugin.Meta.Name,
	})
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
	DataChannel                chan IDataFrame
	waitPublishDone            chan struct{}
	waitStartTime              time.Time
	AudioReader, VideoReader   *AVRingReader
	StartAudioTS, StartVideoTS time.Duration
}

func createSubscriber(p *Plugin, streamPath string, conf config.Subscribe) *Subscriber {
	subscriber := &Subscriber{Subscribe: conf, waitPublishDone: make(chan struct{})}
	subscriber.ID = task.GetNextTaskID()
	subscriber.Plugin = p
	if conf.SubType != "" {
		subscriber.Type = conf.SubType
	} else {
		subscriber.Type = SubscribeTypeServer
	}
	subscriber.Logger = p.Logger.With("streamPath", streamPath, "sId", subscriber.ID)
	subscriber.Init(streamPath, &subscriber.Subscribe)
	if subscriber.Subscribe.BufferTime > 0 {
		subscriber.Subscribe.SubMode = SUBMODE_BUFFER
	}
	return subscriber
}

func (s *Subscriber) waitingPublish() bool {
	return !s.waitStartTime.IsZero()
}

func (s *Subscriber) Start() (err error) {
	server := s.Plugin.Server
	server.Subscribers.Add(s)
	s.Info("subscribe")

	if alias, ok := server.AliasStreams.Get(s.StreamPath); ok {
		if alias.Publisher != nil {
			alias.Publisher.AddSubscriber(s)
			return
		} else {
			server.OnSubscribe(alias.StreamPath, s.Args)
		}
	} else {
		for reg, alias := range server.StreamAlias {
			if streamPath := reg.Replace(s.StreamPath, alias); streamPath != "" {
				server.AliasStreams.Set(&AliasStream{
					StreamPath: streamPath,
					Alias:      s.StreamPath,
				})
				if publisher, ok := server.Streams.Get(streamPath); ok {
					publisher.AddSubscriber(s)
					return
				} else {
					server.OnSubscribe(streamPath, s.Args)
				}
				break
			}
		}
	}
	if publisher, ok := server.Streams.Get(s.StreamPath); ok {
		publisher.AddSubscriber(s)
		return
	} else {
		server.Waiting.Wait(s)
		server.OnSubscribe(s.StreamPath, s.Args)
	}
	return
}

func (s *Subscriber) Dispose() {
	s.Plugin.Server.Subscribers.Remove(s)
	s.Info("unsubscribe", "reason", s.StopReason())
	if s.waitingPublish() {
		s.Plugin.Server.Waiting.Leave(s)
	} else {
		s.Publisher.RemoveSubscriber(s)
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
		if len(b) >= 3 && [3]byte(b[:3]) == [3]byte{'c', 'm', 'd'} {
			pc.Info("control", "cmd", b[3])
			switch b[3] {
			case 1: // pause
				pc.Subscriber.Publisher.Pause()
			case 2: // resume
				pc.Subscriber.Publisher.Resume()
			case 3: // seek
				pc.Subscriber.Publisher.Seek(time.Unix(int64(binary.BigEndian.Uint32(b[4:8])), 0))
			case 4: // speed
				pc.Subscriber.Publisher.Speed = float64(binary.BigEndian.Uint32(b[4:8])) / 100
			case 5: // scale
				pc.Subscriber.Publisher.Scale = float64(binary.BigEndian.Uint32(b[4:8])) / 100
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
	if s.waitingPublish() || dataType == nil {
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
		s.AudioReader = NewAVRingReader(at, dataType.String())
		s.AudioReader.StartTs = startAudioTs
	}
	return
}

func (s *Subscriber) createVideoReader(dataType reflect.Type, startVideoTs time.Duration) (vwi int) {
	if s.waitingPublish() || dataType == nil {
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
		s.VideoReader = NewAVRingReader(vt, dataType.String())
		s.VideoReader.StartTs = startVideoTs
	}
	return
}

type SubscribeHandler[A any, V any] struct {
	//task.Task
	s                            *Subscriber
	p                            *Publisher
	OnAudio                      func(A) error
	OnVideo                      func(V) error
	ProcessAudio, ProcessVideo   chan func(*AVFrame)
	startAudioTs, startVideoTs   time.Duration
	dataTypeAudio, dataTypeVideo reflect.Type
	audioFrame, videoFrame       *AVFrame
	awi, vwi                     int
	lastBPSTime                  time.Time
	bytesRead                    uint32
}

//func Play[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) {
//	s.AddTask(&SubscribeHandler[A, V]{
//		s:       s,
//		OnAudio: onAudio,
//		OnVideo: onVideo,
//	})
//}

func PlayBlock[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) (err error) {
	handler := &SubscribeHandler[A, V]{
		s:       s,
		OnAudio: onAudio,
		OnVideo: onVideo,
	}
	err = handler.Run()
	s.Stop(err)
	return
}

func (handler *SubscribeHandler[A, V]) clearReader() {
	s := handler.s
	if s.AudioReader != nil {
		handler.startAudioTs = time.Duration(s.AudioReader.AbsTime) * time.Millisecond
		s.AudioReader.StopRead()
		s.AudioReader = nil
	}
	if s.VideoReader != nil {
		handler.startVideoTs = time.Duration(s.VideoReader.AbsTime) * time.Millisecond
		s.VideoReader.StopRead()
		s.VideoReader = nil
	}
}

func (handler *SubscribeHandler[A, V]) checkPublishChanged() {
	s := handler.s
	if s.waitingPublish() {
		handler.clearReader()
	}
	if handler.p != s.Publisher {
		handler.clearReader()
		handler.createReaders()
		handler.p = s.Publisher
	}
	runtime.Gosched()
}

func (handler *SubscribeHandler[A, V]) sendAudioFrame() (err error) {
	if handler.awi >= 0 {
		if len(handler.audioFrame.Wraps) > handler.awi {
			if handler.s.Enabled(handler.s, task.TraceLevel) {
				handler.s.Trace("send audio frame", "seq", handler.audioFrame.Sequence)
			}
			err = handler.OnAudio(handler.audioFrame.Wraps[handler.awi].(A))
		} else {
			handler.s.AudioReader.StopRead()
		}
		// Calculate BPS
		if handler.s.AudioReader != nil {
			handler.bytesRead += uint32(handler.audioFrame.Wraps[handler.awi].GetSize())
			now := time.Now()
			if elapsed := now.Sub(handler.lastBPSTime); elapsed >= time.Second {
				handler.s.AudioReader.BPS = uint32(float64(handler.bytesRead) / elapsed.Seconds())
				handler.bytesRead = 0
				handler.lastBPSTime = now
			}
		}
	} else {
		err = handler.OnAudio(any(handler.audioFrame).(A))
	}
	if err != nil && !errors.Is(err, ErrInterrupt) {
		handler.s.Stop(err)
	}
	if handler.ProcessAudio != nil {
		if f, ok := <-handler.ProcessAudio; ok {
			f(handler.audioFrame)
		}
	}
	handler.audioFrame = nil
	return
}

func (handler *SubscribeHandler[A, V]) sendVideoFrame() (err error) {
	if handler.vwi >= 0 {
		if len(handler.videoFrame.Wraps) > handler.vwi {
			if handler.s.Enabled(handler.s, task.TraceLevel) {
				handler.s.Trace("send video frame", "seq", handler.videoFrame.Sequence, "data", handler.videoFrame.Wraps[handler.vwi].String(), "size", handler.videoFrame.Wraps[handler.vwi].GetSize())
			}
			err = handler.OnVideo(handler.videoFrame.Wraps[handler.vwi].(V))
		} else {
			handler.s.VideoReader.StopRead()
		}
		// Calculate BPS
		if handler.s.VideoReader != nil {
			handler.bytesRead += uint32(handler.videoFrame.Wraps[handler.vwi].GetSize())
			now := time.Now()
			if elapsed := now.Sub(handler.lastBPSTime); elapsed >= time.Second {
				handler.s.VideoReader.BPS = uint32(float64(handler.bytesRead) / elapsed.Seconds())
				handler.bytesRead = 0
				handler.lastBPSTime = now
			}
		}
	} else {
		err = handler.OnVideo(any(handler.videoFrame).(V))
	}
	if err != nil && !errors.Is(err, ErrInterrupt) {
		handler.s.Stop(err)
	}
	if handler.ProcessVideo != nil {
		if f, ok := <-handler.ProcessVideo; ok {
			f(handler.videoFrame)
		}
	}
	handler.videoFrame = nil
	return
}

func (handler *SubscribeHandler[A, V]) createReaders() {
	handler.createAudioReader()
	handler.createVideoReader()
}

func (handler *SubscribeHandler[A, V]) createVideoReader() {
	handler.vwi = handler.s.createVideoReader(handler.dataTypeVideo, handler.startVideoTs)
}

func (handler *SubscribeHandler[A, V]) createAudioReader() {
	handler.awi = handler.s.createAudioReader(handler.dataTypeAudio, handler.startAudioTs)
}

func (handler *SubscribeHandler[A, V]) Run() (err error) {
	handler.s.SetDescription("play", time.Now())
	s := handler.s
	handler.startAudioTs, handler.startVideoTs = s.StartAudioTS, s.StartVideoTS
	var initState = 0
	handler.p = s.Publisher
	if s.SubAudio && handler.OnAudio != nil {
		handler.dataTypeAudio = reflect.TypeOf(handler.OnAudio).In(0)
	}
	if s.SubVideo && handler.OnVideo != nil {
		handler.dataTypeVideo = reflect.TypeOf(handler.OnVideo).In(0)
	}
	handler.createReaders()
	handler.lastBPSTime = time.Now()
	defer func() {
		handler.clearReader()
		handler.s.SetDescription("stopPlay", time.Now())
	}()

	for err == nil {
		err = s.Err()
		ar, vr := s.AudioReader, s.VideoReader
		if vr != nil {
			for err == nil {
				err = vr.ReadFrame(&s.Subscribe)
				if handler.p != s.Publisher || s.waitingPublish() {
					break
				}
				if err == nil {
					handler.videoFrame = &vr.Value
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
				if handler.videoFrame.IDR && vr.DecConfChanged() {
					vr.LastCodecCtx = vr.Track.ICodecCtx
					if seqFrame := vr.Track.SequenceFrame; seqFrame != nil {
						s.Debug("video codec changed", "data", seqFrame.String())
						if handler.vwi >= 0 {
							err = handler.OnVideo(seqFrame.(V))
						}
					}
				}
				if ar != nil {
					if handler.audioFrame != nil {
						if util.Conditional(s.SyncMode == 0, handler.videoFrame.Timestamp > handler.audioFrame.Timestamp, handler.videoFrame.WriteTime.After(handler.audioFrame.WriteTime)) {
							// fmt.Println("switch audio", audioFrame.CanRead)
							err = handler.sendAudioFrame()
							break
						}
					} else if initState++; initState >= 2 {
						break
					}
				}

				if !s.IFrameOnly || handler.videoFrame.IDR {
					err = handler.sendVideoFrame()
				}
				if ar == nil {
					break
				}
			}
		} else {
			handler.createVideoReader()
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
					if handler.p != s.Publisher || s.waitingPublish() {
						break
					}
					handler.audioFrame = &ar.Value
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
						if handler.awi >= 0 {
							err = handler.OnAudio(seqFrame.(A))
						}
					}
				}
				if vr != nil && handler.videoFrame != nil {
					if util.Conditional(s.SyncMode == 0, handler.audioFrame.Timestamp > handler.videoFrame.Timestamp, handler.audioFrame.WriteTime.After(handler.videoFrame.WriteTime)) {
						err = handler.sendVideoFrame()
						break
					}
				}
				if handler.audioFrame.Timestamp >= ar.SkipTs {
					err = handler.sendAudioFrame()
				} else {
					s.Debug("skip audio", "frame.AbsTime", handler.audioFrame.Timestamp, "s.AudioReader.SkipTs", ar.SkipTs)
				}
			}
		} else {
			handler.createAudioReader()
		}
		handler.checkPublishChanged()
	}
	return
}
