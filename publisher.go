package m7s

import (
	"reflect"
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
	track.WrapIndex = len(t.Items)
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
	Subscribers map[*Subscriber]struct{} `json:"-" yaml:"-"`
	GOP         int
	baseTs      time.Duration
	lastTs      time.Duration
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

func (p *Publisher) RemoveSubscriber(subscriber *Subscriber) (err error) {
	p.Lock()
	defer p.Unlock()
	delete(p.Subscribers, subscriber)
	p.Info("subscriber -1", "count", len(p.Subscribers))
	if p.State == PublisherStateSubscribed && len(p.Subscribers) == 0 {
		p.State = PublisherStateWaitSubscriber
		if p.DelayCloseTimeout > 0 {
			p.TimeoutTimer.Reset(p.DelayCloseTimeout)
		}
	}
	return
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) (err error) {
	p.Lock()
	defer p.Unlock()
	subscriber.Publisher = p
	if _, ok := p.Subscribers[subscriber]; !ok {
		p.Subscribers[subscriber] = struct{}{}
		p.Info("subscriber +1", "count", len(p.Subscribers))
		switch p.State {
		case PublisherStateTrackAdded, PublisherStateWaitSubscriber:
			p.State = PublisherStateSubscribed
			if p.PublishTimeout > 0 {
				p.TimeoutTimer.Reset(p.PublishTimeout)
			}
		}
	}
	return
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
	if !p.PubVideo || p.IsStopped() {
		return ErrMuted
	}
	t := p.VideoTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "video"), 50)
		p.Lock()
		p.VideoTrack.AVTrack = t
		p.VideoTrack.Add(t)
		if len(p.Subscribers) > 0 {
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
	idr, hidr := t.IDRing.Load(), t.HistoryRing.Load()
	if t.Value.IDR {
		if idr != nil {
			p.GOP = int(t.Value.Sequence - idr.Value.Sequence)
			if hidr == nil {
				if l := t.Size - p.GOP; l > p.GetPublishConfig().MinRingSize {
					t.Debug("resize", "gop", p.GOP, "before", t.Size, "after", t.Size-5)
					t.Reduce(5) //缩小缓冲环节省内存
				}
			}
		}
		if p.BufferTime > 0 {
			t.IDRingList.AddIDR(t.Ring)
			if hidr == nil {
				t.HistoryRing.Store(t.Ring)
			}
		} else {
			t.IDRing.Store(t.Ring)
		}
		if idr == nil {
			p.Info("ready")
			t.Ready.Fulfill(nil)
		}
		if !p.AudioTrack.IsEmpty() {
			p.AudioTrack.IDRing.Store(p.AudioTrack.Ring)
		}
	} else if nextValue := t.Next(); nextValue == idr || nextValue == hidr {
		if t.Size < p.Plugin.GetCommonConf().MaxRingSize {
			t.Glow(5)
		}
	}
	p.writeAV(t, data)
	if p.VideoTrack.Length > 1 && !p.VideoTrack.AVTrack.Ready.IsPending() {
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
				defer track.Ready.Fulfill(err)
			}
			if toFrame, err = track.CreateFrame(&t.Value); err != nil {
				track.Error("from raw", "err", err)
				return
			}
			if codecCtxChanged {
				toFrame.DecodeConfig(track, t.ICodecCtx)
			}
			t.Value.Wraps = append(t.Value.Wraps, toFrame)
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
	if !p.PubAudio || p.IsStopped() {
		return ErrMuted
	}
	t := p.AudioTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "audio"), 256)
		p.Lock()
		p.AudioTrack.AVTrack = t
		p.AudioTrack.Add(t)
		if len(p.Subscribers) > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	_, _, _, err = data.Parse(t)
	if t.ICodecCtx == nil {
		return ErrUnsupportCodec
	}
	if t.Ready.IsPending() {
		t.Ready.Fulfill(err)
	}
	p.writeAV(t, data)
	t.Step()
	p.AudioTrack.speedControl(p.Publish.Speed, p.lastTs)
	return
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
	if p.DataTrack == nil {
		p.DataTrack = &DataTrack{}
		p.DataTrack.Logger = p.Logger.With("track", "data")
		p.Lock()
		if len(p.Subscribers) > 0 {
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

func (p *Publisher) TakeOver(old *Publisher) {
	p.baseTs = old.lastTs
	p.VideoTrack = old.VideoTrack
	p.VideoTrack.ICodecCtx = nil
	p.VideoTrack.Logger = p.Logger.With("track", "video")
	p.AudioTrack = old.AudioTrack
	p.AudioTrack.ICodecCtx = nil
	p.AudioTrack.Logger = p.Logger.With("track", "audio")
	p.DataTrack = old.DataTrack
	p.Subscribers = old.Subscribers
	// for _, track := range p.TransTrack {
	// 	track.ICodecCtx = nil
	// }
}
