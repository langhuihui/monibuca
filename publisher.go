package m7s

import (
	"context"
	"fmt"
	"m7s.live/m7s/v5/pkg/task"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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
	sync.RWMutex
}

func (t *AVTracks) Set(track *AVTrack) {
	t.AVTrack = track
	t.Lock()
	t.Add(track)
	t.Unlock()
}

func (t *AVTracks) SetMinBuffer(start time.Duration) {
	if t.AVTrack == nil {
		return
	}
	t.AVTrack.BufferRange[0] = start
}

func (t *AVTracks) GetOrCreate(dataType reflect.Type) *AVTrack {
	t.Lock()
	defer t.Unlock()
	if track, ok := t.Get(dataType); ok {
		return track
	}
	if t.AVTrack == nil {
		return nil
	}
	return t.CreateSubTrack(dataType)
}

func (t *AVTracks) CheckTimeout(timeout time.Duration) bool {
	if t.AVTrack == nil {
		return false
	}
	return time.Since(t.AVTrack.LastValue.WriteTime) > timeout
}

func (t *AVTracks) CreateSubTrack(dataType reflect.Type) (track *AVTrack) {
	track = NewAVTrack(dataType, t.AVTrack, util.NewPromise(context.TODO()))
	track.WrapIndex = t.Length
	t.Add(track)
	return
}

func (t *AVTracks) Dispose() {
	t.Lock()
	defer t.Unlock()
	for track := range t.Range {
		track.Dispose()
	}
	t.AVTrack = nil
	t.Clear()
}

type Publisher struct {
	PubSubBase
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

// createPublisher -> Start -> WriteAudio/WriteVideo -> Dispose
func createPublisher(p *Plugin, streamPath string, conf config.Publish) (publisher *Publisher) {
	publisher = &Publisher{Publish: conf}
	publisher.ID = task.GetNextTaskID()
	publisher.Plugin = p
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
	publisher.Logger = p.Logger.With("streamPath", streamPath, "pId", publisher.ID)
	publisher.Init(streamPath, &publisher.Publish)
	return
}

func (p *Publisher) Start() (err error) {
	s := p.Plugin.Server
	if oldPublisher, ok := s.Streams.Get(p.StreamPath); ok {
		if p.KickExist {
			p.takeOver(oldPublisher)
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
		for subscriber := range waiting.Range {
			p.AddSubscriber(subscriber)
		}
		s.Waiting.Remove(waiting)
	}
	for plugin := range s.Plugins.Range {
		if plugin.Disabled {
			continue
		}
		onPublish := plugin.GetCommonConf().OnPub
		if plugin.Meta.Pusher != nil {
			for r, pushConf := range onPublish.Push {
				if group := r.FindStringSubmatch(p.StreamPath); group != nil {
					for i, g := range group {
						pushConf.URL = strings.Replace(pushConf.URL, fmt.Sprintf("$%d", i), g, -1)
					}
					plugin.Push(p.StreamPath, pushConf)
				}
			}
		}
		if plugin.Meta.Recorder != nil {
			for r, recConf := range onPublish.Record {
				if group := r.FindStringSubmatch(p.StreamPath); group != nil {
					for i, g := range group {
						recConf.FilePath = strings.Replace(recConf.FilePath, fmt.Sprintf("$%d", i), g, -1)
					}
					plugin.Record(p.StreamPath, recConf)
				}
			}
		}
		if plugin.Meta.Transformer != nil {
			for r, tranConf := range onPublish.Transform {
				if group := r.FindStringSubmatch(p.StreamPath); group != nil {
					for i, g := range group {
						tranConf.Target = strings.Replace(tranConf.Target, fmt.Sprintf("$%d", i), g, -1)
					}
					plugin.Transform(p.StreamPath, tranConf)
				}
			}
		}

		if v, ok := plugin.handler.(IListenPublishPlugin); ok {
			v.OnPublish(p)
		}
	}
	p.AddTask(&PublishTimeout{Publisher: p})
	if p.PublishTimeout > 0 {
		p.AddTask(&PublishNoDataTimeout{Publisher: p})
	}
	return
}

type PublishTimeout struct {
	task.ChannelTask
	Publisher *Publisher
}

func (p *PublishTimeout) Start() error {
	p.SignalChan = p.Publisher.TimeoutTimer.C
	return nil
}

func (p *PublishTimeout) Dispose() {
	p.Publisher.TimeoutTimer.Stop()
}

func (p *PublishTimeout) Tick(any) {
	switch p.Publisher.State {
	case PublisherStateInit:
		if p.Publisher.PublishTimeout > 0 {
			p.Stop(ErrPublishTimeout)
		}
	case PublisherStateTrackAdded:
		if p.Publisher.Publish.IdleTimeout > 0 {
			p.Stop(ErrPublishIdleTimeout)
		}
	case PublisherStateSubscribed:
	case PublisherStateWaitSubscriber:
		if p.Publisher.Publish.DelayCloseTimeout > 0 {
			p.Stop(ErrPublishDelayCloseTimeout)
		}
	}
}

type PublishNoDataTimeout struct {
	task.TickTask
	Publisher *Publisher
}

func (p *PublishNoDataTimeout) GetTickInterval() time.Duration {
	return time.Second * 5
}

func (p *PublishNoDataTimeout) Tick(any) {
	if p.Publisher.VideoTrack.CheckTimeout(p.Publisher.PublishTimeout) {
		p.Error("video timeout", "writeTime", p.Publisher.VideoTrack.LastValue.WriteTime)
		p.Stop(ErrPublishTimeout)
	}
	if p.Publisher.AudioTrack.CheckTimeout(p.Publisher.PublishTimeout) {
		p.Error("audio timeout", "writeTime", p.Publisher.AudioTrack.LastValue.WriteTime)
		p.Stop(ErrPublishTimeout)
	}
}

func (p *Publisher) RemoveSubscriber(subscriber *Subscriber) {
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
	p.AudioTrack.SetMinBuffer(p.BufferTime)
	p.VideoTrack.SetMinBuffer(p.BufferTime)
	if p.State == PublisherStateSubscribed && p.Subscribers.Length == 0 {
		p.State = PublisherStateWaitSubscriber
		if p.DelayCloseTimeout > 0 {
			p.TimeoutTimer.Reset(p.DelayCloseTimeout)
		}
	}
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) {
	subscriber.Publisher = p
	if p.Subscribers.AddUnique(subscriber) {
		p.Info("subscriber +1", "count", p.Subscribers.Length)
		if subscriber.BufferTime > p.BufferTime {
			p.BufferTime = subscriber.BufferTime
			p.AudioTrack.SetMinBuffer(p.BufferTime)
			p.VideoTrack.SetMinBuffer(p.BufferTime)
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
	if p.Enabled(p, task.TraceLevel) {
		codec := t.FourCC().String()
		data := frame.Wraps[0].String()
		p.Trace("write", "seq", frame.Sequence, "ts", uint32(frame.Timestamp/time.Millisecond), "codec", codec, "size", bytesIn, "data", data)
	}
}

func (p *Publisher) trackAdded() error {
	if p.Subscribers.Length > 0 {
		p.State = PublisherStateSubscribed
	} else {
		p.State = PublisherStateTrackAdded
	}
	return nil
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
		p.VideoTrack.Set(t)
		p.Call(p.trackAdded)
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
		p.AudioTrack.Set(t)
		p.Call(p.trackAdded)
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
		p.Call(p.trackAdded)
	}
	// TODO: Implement this function
	return
}

func (p *Publisher) GetAudioTrack(dataType reflect.Type) (t *AVTrack) {
	return p.AudioTrack.GetOrCreate(dataType)
}

func (p *Publisher) GetVideoTrack(dataType reflect.Type) (t *AVTrack) {
	return p.VideoTrack.GetOrCreate(dataType)
}

func (p *Publisher) HasAudioTrack() bool {
	return p.AudioTrack.Length > 0
}

func (p *Publisher) HasVideoTrack() bool {
	return p.VideoTrack.Length > 0
}

func (p *Publisher) Dispose() {
	s := p.Plugin.Server
	if !p.StopReasonIs(ErrKick) {
		s.Streams.Remove(p)
	}
	if p.Subscribers.Length > 0 {
		w := s.createWait(p.StreamPath)
		w.baseTs = p.lastTs
		w.Info("takeOver", "pId", p.ID)
		for subscriber := range p.SubscriberRange {
			subscriber.Publisher = nil
			w.Add(subscriber)
		}
		p.AudioTrack.Dispose()
		p.VideoTrack.Dispose()
		p.Subscribers.Clear()
	}
	p.Info("unpublish", "remain", s.Streams.Length, "reason", p.StopReason())
	if p.dumpFile != nil {
		p.dumpFile.Close()
	}
	p.State = PublisherStateDisposed
}

func (p *Publisher) takeOver(old *Publisher) {
	p.baseTs = old.lastTs
	old.Stop(ErrKick)
	p.Info("takeOver", "old", old.ID)
	for subscriber := range old.SubscriberRange {
		p.AddSubscriber(subscriber)
	}
	old.AudioTrack.Dispose()
	old.VideoTrack.Dispose()
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

func (p *Publisher) Pause() {
	//p.AudioTrack.Pause()
	//p.VideoTrack.Pause()
}

func (p *Publisher) Resume() {
	//p.AudioTrack.Resume()
	//p.VideoTrack.Resume()
}

func (p *Publisher) FastForward() {
	//p.AudioTrack.FastForward()
	//p.VideoTrack.FastForward()
}
