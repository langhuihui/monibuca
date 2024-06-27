package m7s

import (
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
		if s.Delta > time.Second {
			time.Sleep(s.Delta)
		}
	}
}

type AVTracks struct {
	*AVTrack
	SpeedControl
	util.Collection[reflect.Type, *AVTrack]
}

func (t *AVTracks) IsEmpty() bool {
	return t.Length == 0
}

func (t *AVTracks) CreateSubTrack(dataType reflect.Type) (track *AVTrack) {
	track = NewAVTrack(dataType, t.AVTrack)
	track.WrapIndex = t.Length
	t.Add(track)
	return
}

type Publisher struct {
	PubSubBase
	sync.RWMutex `json:"-" yaml:"-"`
	config.Publish
	State       PublisherState
	VideoTrack  AVTracks
	AudioTrack  AVTracks
	DataTrack   *DataTrack
	Subscribers util.Collection[int, *Subscriber] `json:"-" yaml:"-"`
	GOP         int
	baseTs      time.Duration
	lastTs      time.Duration
	dumpFile    *os.File
}

func (p *Publisher) SubscriberRange(yield func(sub *Subscriber) bool) {
	p.Subscribers.Range(yield)
}

func (p *Publisher) GetKey() string {
	return p.StreamPath
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
			if !p.VideoTrack.IsEmpty() && !p.VideoTrack.LastValue.WriteTime.IsZero() && time.Since(p.VideoTrack.LastValue.WriteTime) > p.PublishTimeout {
				p.Error("video timeout", "writeTime", p.VideoTrack.LastValue.WriteTime)
				err = ErrPublishTimeout
			}
			if !p.AudioTrack.IsEmpty() && !p.AudioTrack.LastValue.WriteTime.IsZero() && time.Since(p.AudioTrack.LastValue.WriteTime) > p.PublishTimeout {
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
	if !p.AudioTrack.IsEmpty() {
		p.AudioTrack.AVTrack.BufferRange[0] = p.BufferTime
	}
	if !p.VideoTrack.IsEmpty() {
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
			if !p.AudioTrack.IsEmpty() {
				p.AudioTrack.AVTrack.BufferRange[0] = p.BufferTime
			}
			if !p.VideoTrack.IsEmpty() {
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

func (p *Publisher) Start() {
	p.Info("publish")
	if p.Dump {
		f := filepath.Join("./dump", p.StreamPath)
		os.MkdirAll(filepath.Dir(f), 0666)
		p.dumpFile, _ = os.OpenFile(filepath.Join("./dump", p.StreamPath), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	}
}

func (p *Publisher) writeAV(t *AVTrack, data IAVFrame) {
	frame := &t.Value
	frame.Wraps = append(frame.Wraps, data)
	ts := data.GetTimestamp()
	if p.lastTs == 0 {
		p.baseTs -= ts
	}
	frame.Timestamp = max(1, p.baseTs+ts)
	bytesIn := frame.Wraps[0].GetSize()
	t.AddBytesIn(bytesIn)
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
	if p.dumpFile != nil {
		data.Dump(1, p.dumpFile)
	}
	if !p.PubVideo || p.IsStopped() {
		return ErrMuted
	}
	t := p.VideoTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "video"), &p.Publish)
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
	t.Value.IDR, _, t.Value.Raw, err = data.Parse(t)
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
			p.Info("ready")
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
			t.Value.Raw, err = t.Value.Wraps[0].ToRaw(t.ICodecCtx)
			if err != nil {
				t.Error("to raw", "err", err)
				return err
			}
		}
		var toFrame IAVFrame
		for i, track := range p.VideoTrack.Items[1:] {
			if track.ICodecCtx == nil {
				err = (reflect.New(track.FrameType.Elem()).Interface().(IAVFrame)).DecodeConfig(track, t.ICodecCtx)
				if err != nil {
					track.Error("DecodeConfig", "err", err)
					return
				}
				if t.IDRingList.Len() > 0 {
					for rf := t.IDRingList.Front().Value; rf != t.Ring; rf = rf.Next() {
						if i == 0 && rf.Value.Raw == nil {
							rf.Value.Raw, err = rf.Value.Wraps[0].ToRaw(t.ICodecCtx)
							if err != nil {
								t.Error("to raw", "err", err)
								return err
							}
						}
						if toFrame, err = track.CreateFrame(&rf.Value); err != nil {
							track.Error("from raw", "err", err)
							return
						}
						rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
					}
				}
			}
			if toFrame, err = track.CreateFrame(&t.Value); err != nil {
				track.Error("from raw", "err", err)
				return
			}
			if codecCtxChanged {
				toFrame.DecodeConfig(track, t.ICodecCtx)
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
	if p.dumpFile != nil {
		data.Dump(0, p.dumpFile)
	}
	if !p.PubAudio || p.IsStopped() {
		return ErrMuted
	}
	t := p.AudioTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "audio"), &p.Publish)
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
	_, _, t.Value.Raw, err = data.Parse(t)
	codecCtxChanged := oldCodecCtx != t.ICodecCtx
	if t.ICodecCtx == nil {
		return ErrUnsupportCodec
	}
	t.Ready(err)
	p.writeAV(t, data)
	if p.AudioTrack.Length > 1 && p.AudioTrack.IsReady() {
		if t.Value.Raw == nil {
			t.Value.Raw, err = t.Value.Wraps[0].ToRaw(t.ICodecCtx)
			if err != nil {
				t.Error("to raw", "err", err)
				return err
			}
		}
		var toFrame IAVFrame
		for i, track := range p.AudioTrack.Items[1:] {
			if track.ICodecCtx == nil {
				err = (reflect.New(track.FrameType.Elem()).Interface().(IAVFrame)).DecodeConfig(track, t.ICodecCtx)
				if err != nil {
					track.Error("DecodeConfig", "err", err)
					return
				}
				if idr := p.AudioTrack.GetOldestIDR(); idr != nil {
					for rf := idr; rf != t.Ring; rf = rf.Next() {
						if i == 0 && rf.Value.Raw == nil {
							rf.Value.Raw, err = rf.Value.Wraps[0].ToRaw(t.ICodecCtx)
							if err != nil {
								t.Error("to raw", "err", err)
								return err
							}
						}
						if toFrame, err = track.CreateFrame(&rf.Value); err != nil {
							track.Error("from raw", "err", err)
							return
						}
						rf.Value.Wraps = append(rf.Value.Wraps, toFrame)
					}
				}
			}
			if toFrame, err = track.CreateFrame(&t.Value); err != nil {
				track.Error("from raw", "err", err)
				return
			}
			if codecCtxChanged {
				toFrame.DecodeConfig(track, t.ICodecCtx)
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
	if !p.AudioTrack.IsEmpty() {
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
	if !p.VideoTrack.IsEmpty() {
		return p.VideoTrack.CreateSubTrack(dataType)
	}
	return
}

func (p *Publisher) Dispose(err error) {
	p.Lock()
	defer p.Unlock()
	if p.dumpFile != nil {
		p.dumpFile.Close()
	}
	if p.State == PublisherStateDisposed {
		return
	}
	if p.IsStopped() {
		if !p.AudioTrack.IsEmpty() {
			p.AudioTrack.Dispose()
		}
		if !p.VideoTrack.IsEmpty() {
			p.VideoTrack.Dispose()
		}
		p.State = PublisherStateDisposed
		return
	}
	p.Stop(err)
}

func (p *Publisher) TakeOver(old *Publisher) {
	p.baseTs = old.lastTs
	p.Info("takeOver", "old", old.ID)
	for subscriber := range old.SubscriberRange {
		p.AddSubscriber(subscriber)
	}
	if old.Plugin != nil {
		old.Dispose(nil)
	}
	old.Subscribers = util.Collection[int, *Subscriber]{}
}
