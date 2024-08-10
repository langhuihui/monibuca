package m7s

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type PublisherState int

const (
	PublisherStateInit PublisherState = iota
	PublisherStateTrackAdded
	PublisherStateSubscribed
	PublisherStateWaitSubscriber
	PublisherStateDisposed
)

const threshold = 10 * time.Millisecond

type SpeedControl struct {
	speed          float64
	beginTime      time.Time
	beginTimestamp time.Duration
	Delta          time.Duration
}

func (s *SpeedControl) speedControl(speed float64, ts time.Duration) {
	if speed != s.speed || s.beginTime.IsZero() {
		s.speed = speed
		s.beginTime = time.Now()
		s.beginTimestamp = ts
	} else {
		elapsed := time.Since(s.beginTime)
		if speed == 0 {
			s.Delta = ts - elapsed
			return
		}
		should := time.Duration(float64(ts) / speed)
		s.Delta = should - elapsed
		if s.Delta > threshold {
			time.Sleep(s.Delta)
		}
	}
}

type AVTracks struct {
	*AVTrack
	SpeedControl
	util.Collection[reflect.Type, *AVTrack]
}

func (t *AVTracks) CreateSubTrack(dataType reflect.Type) (track *AVTrack) {
	track = NewAVTrack(dataType, t.AVTrack, util.NewPromise(context.TODO()))
	track.WrapIndex = t.Length
	t.Add(track)
	return
}

// createPublisher -> Start -> WriteAudio/WriteVideo -> Dispose
type Publisher struct {
	PubSubBase
	sync.RWMutex
	config.Publish
	State                  PublisherState
	AudioTrack, VideoTrack AVTracks
	audioReady, videoReady *util.Promise
	DataTrack              *DataTrack
	Subscribers            SubscriberCollection
	GOP                    int
	baseTs, lastTs         time.Duration
	dumpFile               *os.File
}

func (p *Publisher) SubscriberRange(yield func(sub *Subscriber) bool) {
	p.Subscribers.Range(yield)
}

func (p *Publisher) GetKey() string {
	return p.StreamPath
}

func createPublisher(p *Plugin, streamPath string, options ...any) (publisher *Publisher) {
	publisher = &Publisher{Publish: p.config.Publish}
	publisher.ID = p.Server.streamTask.GetID()
	publisher.Plugin = p
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
	var opt = []any{publisher, p.Logger.With("streamPath", streamPath, "pId", publisher.ID)}
	for _, option := range options {
		switch v := option.(type) {
		case func(*config.Publish):
			v(&publisher.Publish)
		default:
			opt = append(opt, option)
		}
	}
	publisher.Init(streamPath, &publisher.Publish, opt...)
	return
}

func (p *Publisher) Start() (err error) {
	s := p.Plugin.Server
	if oldPublisher, ok := s.Streams.Get(p.StreamPath); ok {
		if p.KickExist {
			p.Warn("kick")
			oldPublisher.Stop(ErrKick)
			p.TakeOver(oldPublisher)
		} else {
			return ErrStreamExist
		}
	}
	s.Streams.Set(p)
	p.Info("publish")
	p.audioReady = util.NewPromiseWithTimeout(p, time.Second*5)
	p.videoReady = util.NewPromiseWithTimeout(p, time.Second*5)
	if p.Dump {
		f := filepath.Join("./dump", p.StreamPath)
		os.MkdirAll(filepath.Dir(f), 0666)
		p.dumpFile, _ = os.OpenFile(filepath.Join("./dump", p.StreamPath), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	}
	if waiting, ok := s.Waiting.Get(p.StreamPath); ok {
		p.TakeOver(waiting)
		s.Waiting.Remove(waiting)
	}
	for plugin := range s.Plugins.Range {
		if plugin.Disabled {
			continue
		}
		if remoteURL := plugin.GetCommonConf().CheckPush(p.StreamPath); remoteURL != "" {
			if plugin.Meta.Pusher != nil {
				plugin.Push(p.StreamPath, remoteURL, plugin.Meta.Pusher)
			}
		}
		if filePath := plugin.GetCommonConf().CheckRecord(p.StreamPath); filePath != "" {
			if plugin.Meta.Recorder != nil {
				go plugin.RecordBlock(p.StreamPath, filePath, plugin.Meta.Recorder)
			}
		}
		//if h, ok := plugin.handler.(IOnPublishPlugin); ok {
		//	h.OnPublish(publisher)
		//}
	}
	return
}

func (p *Publisher) timeout() (err error) {
	switch p.State {
	case PublisherStateInit:
		if p.PublishTimeout > 0 {
			err = ErrPublishTimeout
		}
	case PublisherStateTrackAdded:
		if p.Publish.IdleTimeout > 0 {
			err = ErrPublishIdleTimeout
		}
	case PublisherStateSubscribed:
	case PublisherStateWaitSubscriber:
		if p.Publish.DelayCloseTimeout > 0 {
			err = ErrPublishDelayCloseTimeout
		}
	}
	return
}

func (p *Publisher) checkTimeout() (err error) {
	select {
	case <-p.TimeoutTimer.C:
		err = p.timeout()
	default:
		if p.PublishTimeout > 0 {
			if p.HasVideoTrack() && !p.VideoTrack.LastValue.WriteTime.IsZero() && time.Since(p.VideoTrack.LastValue.WriteTime) > p.PublishTimeout {
				p.Error("video timeout", "writeTime", p.VideoTrack.LastValue.WriteTime)
				err = ErrPublishTimeout
			}
			if p.HasAudioTrack() && !p.AudioTrack.LastValue.WriteTime.IsZero() && time.Since(p.AudioTrack.LastValue.WriteTime) > p.PublishTimeout {
				p.Error("audio timeout", "writeTime", p.AudioTrack.LastValue.WriteTime)
				err = ErrPublishTimeout
			}
		}
	}
	return
}

func (p *Publisher) RemoveSubscriber(subscriber *Subscriber) {
	p.Lock()
	defer p.Unlock()
	p.Subscribers.Remove(subscriber)
	p.Info("subscriber -1", "count", p.Subscribers.Length)
	if p.Plugin == nil {
		return
	}
	if subscriber.BufferTime == p.BufferTime && p.Subscribers.Length > 0 {
		p.BufferTime = slices.MaxFunc(p.Subscribers.Items, func(a, b *Subscriber) int {
			return int(a.BufferTime - b.BufferTime)
		}).BufferTime
	} else {
		p.BufferTime = p.Plugin.GetCommonConf().Publish.BufferTime
	}
	if p.HasAudioTrack() {
		p.AudioTrack.AVTrack.BufferRange[0] = p.BufferTime
	}
	if p.HasVideoTrack() {
		p.VideoTrack.AVTrack.BufferRange[0] = p.BufferTime
	}
	if p.State == PublisherStateSubscribed && p.Subscribers.Length == 0 {
		p.State = PublisherStateWaitSubscriber
		if p.DelayCloseTimeout > 0 {
			p.TimeoutTimer.Reset(p.DelayCloseTimeout)
		}
	}
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) {
	p.Lock()
	defer p.Unlock()
	subscriber.Publisher = p
	if p.Subscribers.AddUnique(subscriber) {
		p.Info("subscriber +1", "count", p.Subscribers.Length)
		if subscriber.BufferTime > p.BufferTime {
			p.BufferTime = subscriber.BufferTime
			if p.HasAudioTrack() {
				p.AudioTrack.AVTrack.BufferRange[0] = p.BufferTime
			}
			if p.HasVideoTrack() {
				p.VideoTrack.AVTrack.BufferRange[0] = p.BufferTime
			}
		}
		switch p.State {
		case PublisherStateTrackAdded, PublisherStateWaitSubscriber:
			p.State = PublisherStateSubscribed
			if p.PublishTimeout > 0 {
				p.TimeoutTimer.Reset(p.PublishTimeout)
			}
		}
	}
}

func (p *Publisher) writeAV(t *AVTrack, data IAVFrame) {
	frame := &t.Value
	frame.Wraps = append(frame.Wraps, data)
	ts := data.GetTimestamp()
	frame.CTS = data.GetCTS()
	if p.lastTs == 0 {
		p.baseTs -= ts
	}
	frame.Timestamp = max(1, p.baseTs+ts)
	bytesIn := frame.Wraps[0].GetSize()
	t.AddBytesIn(bytesIn)
	if t.FPS > 0 {
		frameDur := float64(time.Second) / float64(t.FPS)
		if math.Abs(float64(frame.Timestamp-p.lastTs)) > 5*frameDur { //时间戳突变
			frame.Timestamp = p.lastTs + time.Duration(frameDur)
			p.baseTs = frame.Timestamp - ts
		}
	}
	p.lastTs = frame.Timestamp
	if p.Enabled(p, TraceLevel) {
		codec := t.FourCC().String()
		data := frame.Wraps[0].String()
		p.Trace("write", "seq", frame.Sequence, "ts", uint32(frame.Timestamp/time.Millisecond), "codec", codec, "size", bytesIn, "data", data)
	}
}

func (p *Publisher) WriteVideo(data IAVFrame) (err error) {
	defer func() {
		if err != nil {
			data.Recycle()
		}
	}()
	if err = p.Err(); err != nil {
		return
	}
	if p.dumpFile != nil {
		data.Dump(1, p.dumpFile)
	}
	if !p.PubVideo {
		return ErrMuted
	}
	t := p.VideoTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "video"), &p.Publish, p.videoReady)
		p.Lock()
		p.VideoTrack.AVTrack = t
		p.VideoTrack.Add(t)
		if p.Subscribers.Length > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	oldCodecCtx := t.ICodecCtx
	err = data.Parse(t)
	codecCtxChanged := oldCodecCtx != t.ICodecCtx
	if err != nil {
		p.Error("parse", "err", err)
		return err
	}
	if t.ICodecCtx == nil {
		return ErrUnsupportCodec
	}
	var idr *util.Ring[AVFrame]
	if t.IDRingList.Len() > 0 {
		idr = t.IDRingList.Back().Value
	}
	if t.Value.IDR {
		if !t.IsReady() {
			t.Ready(nil)
		} else if idr != nil {
			p.GOP = int(t.Value.Sequence - idr.Value.Sequence)
		} else {
			p.GOP = 0
		}
		if p.AudioTrack.Length > 0 {
			p.AudioTrack.PushIDR()
		}
	}
	p.writeAV(t, data)
	if p.VideoTrack.Length > 1 && p.VideoTrack.IsReady() {
		if t.Value.Raw == nil {
			if err = t.Value.Demux(t.ICodecCtx); err != nil {
				t.Error("to raw", "err", err)
				return err
			}
		}
		for i, track := range p.VideoTrack.Items[1:] {
			toType := track.FrameType.Elem()
			toFrame := reflect.New(toType).Interface().(IAVFrame)
			if track.ICodecCtx == nil {
				if track.ICodecCtx, track.SequenceFrame, err = toFrame.ConvertCtx(t.ICodecCtx); err != nil {
					track.Error("DecodeConfig", "err", err)
					return
				}
				if t.IDRingList.Len() > 0 {
					for rf := t.IDRingList.Front().Value; rf != t.Ring; rf = rf.Next() {
						if i == 0 && rf.Value.Raw == nil {
							if err = rf.Value.Demux(t.ICodecCtx); err != nil {
								t.Error("to raw", "err", err)
								return err
							}
						}
						toFrame := reflect.New(toType).Interface().(IAVFrame)
						toFrame.SetAllocator(data.GetAllocator())
						toFrame.Mux(track.ICodecCtx, &rf.Value)
						rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
					}
				}
			}
			toFrame.SetAllocator(data.GetAllocator())
			toFrame.Mux(track.ICodecCtx, &t.Value)
			if codecCtxChanged {
				track.ICodecCtx, track.SequenceFrame, err = toFrame.ConvertCtx(t.ICodecCtx)
			}
			t.Value.Wraps = append(t.Value.Wraps, toFrame)
			if track.ICodecCtx != nil {
				track.Ready(err)
			}
		}
	}
	t.Step()
	p.VideoTrack.speedControl(p.Speed, p.lastTs)
	return
}

func (p *Publisher) WriteAudio(data IAVFrame) (err error) {
	defer func() {
		if err != nil {
			data.Recycle()
		}
	}()
	if err = p.Err(); err != nil {
		return
	}
	if p.dumpFile != nil {
		data.Dump(0, p.dumpFile)
	}
	if !p.PubAudio {
		return ErrMuted
	}
	t := p.AudioTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "audio"), &p.Publish, p.audioReady)
		p.Lock()
		p.AudioTrack.AVTrack = t
		p.AudioTrack.Add(t)
		if p.Subscribers.Length > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	oldCodecCtx := t.ICodecCtx
	err = data.Parse(t)
	codecCtxChanged := oldCodecCtx != t.ICodecCtx
	if t.ICodecCtx == nil {
		return ErrUnsupportCodec
	}
	t.Ready(err)
	p.writeAV(t, data)
	if p.AudioTrack.Length > 1 && p.AudioTrack.IsReady() {
		if t.Value.Raw == nil {
			if err = t.Value.Demux(t.ICodecCtx); err != nil {
				t.Error("to raw", "err", err)
				return err
			}
		}
		for i, track := range p.AudioTrack.Items[1:] {
			toType := track.FrameType.Elem()
			toFrame := reflect.New(toType).Interface().(IAVFrame)
			if track.ICodecCtx == nil {
				if track.ICodecCtx, track.SequenceFrame, err = toFrame.ConvertCtx(t.ICodecCtx); err != nil {
					track.Error("DecodeConfig", "err", err)
					return
				}
				if idr := p.AudioTrack.GetOldestIDR(); idr != nil {
					for rf := idr; rf != t.Ring; rf = rf.Next() {
						if i == 0 && rf.Value.Raw == nil {
							if err = rf.Value.Demux(t.ICodecCtx); err != nil {
								t.Error("to raw", "err", err)
								return err
							}
						}
						toFrame := reflect.New(toType).Interface().(IAVFrame)
						toFrame.SetAllocator(data.GetAllocator())
						toFrame.Mux(track.ICodecCtx, &rf.Value)
						rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
					}
				}
			}
			toFrame.SetAllocator(data.GetAllocator())
			toFrame.Mux(track.ICodecCtx, &t.Value)
			if codecCtxChanged {
				track.ICodecCtx, track.SequenceFrame, err = toFrame.ConvertCtx(t.ICodecCtx)
			}
			t.Value.Wraps = append(t.Value.Wraps, toFrame)
			if track.ICodecCtx != nil {
				track.Ready(err)
			}
		}
	}
	t.Step()
	p.AudioTrack.speedControl(p.Publish.Speed, p.lastTs)
	return
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
	if err = p.Err(); err != nil {
		return
	}
	if p.DataTrack == nil {
		p.DataTrack = &DataTrack{}
		p.DataTrack.Logger = p.Logger.With("track", "data")
		p.Lock()
		if p.Subscribers.Length > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	// TODO: Implement this function
	return
}

func (p *Publisher) GetAudioTrack(dataType reflect.Type) (t *AVTrack) {
	p.Lock()
	defer p.Unlock()
	if t, ok := p.AudioTrack.Get(dataType); ok {
		return t
	}
	if p.HasAudioTrack() {
		return p.AudioTrack.CreateSubTrack(dataType)
	}
	return
}

func (p *Publisher) GetVideoTrack(dataType reflect.Type) (t *AVTrack) {
	p.Lock()
	defer p.Unlock()
	if t, ok := p.VideoTrack.Get(dataType); ok {
		return t
	}
	if p.HasVideoTrack() {
		return p.VideoTrack.CreateSubTrack(dataType)
	}
	return
}

func (p *Publisher) HasAudioTrack() bool {
	return p.AudioTrack.Length > 0
}

func (p *Publisher) HasVideoTrack() bool {
	return p.VideoTrack.Length > 0
}

func (p *Publisher) Dispose() {
	s := p.Plugin.Server
	s.Streams.Remove(p)
	if p.Subscribers.Length > 0 {
		s.Waiting.Add(p)
	}
	p.Info("unpublish", "remain", s.Streams.Length, "reason", p.StopReason())
	for subscriber := range p.SubscriberRange {
		waitCloseTimeout := p.WaitCloseTimeout
		if waitCloseTimeout == 0 {
			waitCloseTimeout = subscriber.WaitTimeout
		}
		subscriber.TimeoutTimer.Reset(waitCloseTimeout)
	}
	p.Lock()
	defer p.Unlock()
	if p.dumpFile != nil {
		p.dumpFile.Close()
	}
	if p.State == PublisherStateDisposed {
		panic("disposed")
	}
	if p.HasAudioTrack() {
		p.AudioTrack.Dispose()
	}
	if p.HasVideoTrack() {
		p.VideoTrack.Dispose()
	}
	p.State = PublisherStateDisposed
}

func (p *Publisher) TakeOver(old *Publisher) {
	p.baseTs = old.lastTs
	p.Info("takeOver", "old", old.ID)
	for subscriber := range old.SubscriberRange {
		p.AddSubscriber(subscriber)
	}
	old.Stop(ErrKick)
	old.Subscribers = SubscriberCollection{}
}

func (p *Publisher) WaitTrack() (err error) {
	if p.PubVideo {
		err = p.videoReady.Await()
	}
	if p.PubAudio {
		err = p.audioReady.Await()
	}
	return
}
