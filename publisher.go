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

	"m7s.live/m7s/v5/pkg/task"

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
		//fmt.Println(speed, elapsed, should, s.Delta)
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
	baseTs time.Duration //from old publisher's lastTs
}

func (t *AVTracks) Set(track *AVTrack) {
	t.AVTrack = track
	track.BaseTs = t.baseTs
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
		if track == t.AVTrack || track.RingWriter != t.AVTrack.RingWriter {
			track.Dispose()
		}
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
		plugin.OnPublish(p)
	}
	s.Transforms.Post(func() error {
		if m, ok := s.Transforms.Transformed.Get(p.StreamPath); ok {
			m.TransformJob.TransformPublished(p)
		}
		return nil
	})
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
			p.Publisher.Stop(ErrPublishTimeout)
		}
	case PublisherStateTrackAdded:
		if p.Publisher.Publish.IdleTimeout > 0 {
			p.Publisher.Stop(ErrPublishIdleTimeout)
		}
	case PublisherStateSubscribed:
	case PublisherStateWaitSubscriber:
		if p.Publisher.Publish.DelayCloseTimeout > 0 {
			p.Publisher.Stop(ErrPublishDelayCloseTimeout)
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
		p.Publisher.Stop(ErrPublishTimeout)
	}
	if p.Publisher.AudioTrack.CheckTimeout(p.Publisher.PublishTimeout) {
		p.Error("audio timeout", "writeTime", p.Publisher.AudioTrack.LastValue.WriteTime)
		p.Publisher.Stop(ErrPublishTimeout)
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
	subscriber.waitPublishDone.Resolve()
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
	if t.LastTs == 0 {
		t.BaseTs -= ts
	}
	frame.Timestamp = max(1, t.BaseTs+ts)
	bytesIn := frame.Wraps[0].GetSize()
	t.AddBytesIn(bytesIn)
	if t.FPS > 0 {
		frameDur := float64(time.Second) / float64(t.FPS)
		if math.Abs(float64(frame.Timestamp-t.LastTs)) > 10*frameDur { //时间戳突变
			p.Warn("timestamp mutation", "fps", t.FPS, "lastTs", t.LastTs, "ts", frame.Timestamp, "frameDur", time.Duration(frameDur))
			frame.Timestamp = t.LastTs + time.Duration(frameDur)
			t.BaseTs = frame.Timestamp - ts
		}
	}
	t.LastTs = frame.Timestamp
	if p.Enabled(p, task.TraceLevel) {
		codec := t.FourCC().String()
		data := frame.Wraps[0].String()
		p.Trace("write", "seq", frame.Sequence, "ts0", ts, "ts", uint32(frame.Timestamp/time.Millisecond), "codec", codec, "size", bytesIn, "data", data)
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
	p.VideoTrack.speedControl(p.Speed, t.LastTs)
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
	p.AudioTrack.speedControl(p.Publish.Speed, t.LastTs)
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
		if p.HasAudioTrack() {
			w.baseTsAudio = p.AudioTrack.LastTs
		}
		if p.HasVideoTrack() {
			w.baseTsVideo = p.VideoTrack.LastTs
		}
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
	if old.HasAudioTrack() {
		p.AudioTrack.baseTs = old.AudioTrack.LastTs
	}
	if old.HasVideoTrack() {
		p.VideoTrack.baseTs = old.VideoTrack.LastTs
	}
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
	var v, a error
	if p.PubVideo {
		v = p.videoReady.Await()
	}
	if p.PubAudio {
		a = p.audioReady.Await()
	}
	if v != nil && a != nil {
		return ErrNoTrack
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
