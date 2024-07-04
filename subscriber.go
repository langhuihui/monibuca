package m7s

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var AVFrameType = reflect.TypeOf((*AVFrame)(nil))

type Owner struct {
	Conn     net.Conn
	File     *os.File
	MetaData any
	io.Closer
}

type PubSubBase struct {
	Unit
	ID int
	Owner
	Plugin       *Plugin
	StreamPath   string
	Args         url.Values
	TimeoutTimer *time.Timer
}

func (p *PubSubBase) GetKey() int {
	return p.ID
}

func (ps *PubSubBase) Init(p *Plugin, streamPath string, conf any, options ...any) {
	ps.Plugin = p
	ctx := p.Context
	for _, option := range options {
		switch v := option.(type) {
		case context.Context:
			ctx = v
		case net.Conn:
			ps.Conn = v
			ps.Closer = v
		case *os.File:
			ps.File = v
			ps.Closer = v
		case io.Closer:
			ps.Closer = v
		default:
			ps.MetaData = v
		}
	}
	ps.Context, ps.CancelCauseFunc = context.WithCancelCause(ctx)
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
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
	ps.StartTime = time.Now()
}

type Subscriber struct {
	PubSubBase
	config.Subscribe
	Publisher   *Publisher
	AudioReader *AVRingReader
	VideoReader *AVRingReader
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

func PlayBlock[A any, V any](s *Subscriber, onAudio func(A) error, onVideo func(V) error) (err error) {
	var a1, v1 reflect.Type
	var startAudioTs, startVideoTs time.Duration
	var initState = 0
	prePublisher := s.Publisher
	var audioFrame, videoFrame *AVFrame
	if s.SubAudio {
		a1 = reflect.TypeOf(onAudio).In(0)
	}
	if s.SubVideo {
		v1 = reflect.TypeOf(onVideo).In(0)
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
				if s.Enabled(s, TraceLevel) {
					s.Trace("send audio frame", "seq", audioFrame.Sequence)
				}
				err = onAudio(audioFrame.Wraps[awi].(A))
			} else {
				s.AudioReader.StopRead()
			}
		} else {
			err = onAudio(any(audioFrame).(A))
		}
		if err != nil && !errors.Is(err, ErrInterrupt) {
			s.Stop(err)
		}
		audioFrame = nil
		return
	}
	sendVideoFrame := func() (err error) {
		if vwi >= 0 {
			if len(videoFrame.Wraps) > vwi {
				if s.Enabled(s, TraceLevel) {
					s.Trace("send video frame", "seq", videoFrame.Sequence, "data", videoFrame.Wraps[vwi].String(), "size", videoFrame.Wraps[vwi].GetSize())
				}
				err = onVideo(videoFrame.Wraps[vwi].(V))
			} else {
				s.VideoReader.StopRead()
			}
		} else {
			err = onVideo(any(videoFrame).(V))
		}
		if err != nil && !errors.Is(err, ErrInterrupt) {
			s.Stop(err)
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
						err = onVideo(seqFrame.(V))
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
						err = onAudio(seqFrame.(A))
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
