package m7s

import (
	"reflect"
	"sync"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/pb"
)

type PublisherState int

const (
	PublisherStateInit PublisherState = iota
	PublisherStateTrackAdded
	PublisherStateSubscribed
	PublisherStateWaitSubscriber
)

type Publisher struct {
	PubSubBase
	sync.RWMutex `json:"-" yaml:"-"`
	config.Publish
	State       PublisherState
	VideoTrack  *AVTrack
	AudioTrack  *AVTrack
	DataTrack   *DataTrack
	TransTrack  map[reflect.Type]*AVTrack `json:"-" yaml:"-"`
	Subscribers map[*Subscriber]struct{}  `json:"-" yaml:"-"`
	GOP         int
	baseTs      time.Duration
	lastTs      time.Duration
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
			if p.VideoTrack != nil && !p.VideoTrack.LastValue.WriteTime.IsZero() && time.Since(p.VideoTrack.LastValue.WriteTime) > p.PublishTimeout {
				err = ErrPublishTimeout
			}
			if p.AudioTrack != nil && !p.AudioTrack.LastValue.WriteTime.IsZero() && time.Since(p.AudioTrack.LastValue.WriteTime) > p.PublishTimeout {
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
	frame.Wrap = data
	ts := data.GetTimestamp()
	if p.lastTs == 0 {
		p.baseTs -= ts
	}
	frame.Timestamp = max(1, p.baseTs+ts)
	p.lastTs = frame.Timestamp
	p.Debug("write", "seq", frame.Sequence)
	t.Step()
}

func (p *Publisher) WriteVideo(data IAVFrame) (err error) {
	if !p.PubVideo || p.IsStopped() {
		return
	}
	t := p.VideoTrack
	if t == nil {
		t = &AVTrack{}
		t.Logger = p.Logger.With("track", "video")
		t.Init(256)
		p.Lock()
		p.VideoTrack = t
		p.TransTrack[reflect.TypeOf(data)] = t
		if len(p.Subscribers) > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	if t.ICodecCtx == nil {
		return data.DecodeConfig(t)
	}
	if data.IsIDR() {
		if t.IDRing != nil {
			p.GOP = int(t.Value.Sequence - t.IDRing.Value.Sequence)
			if t.HistoryRing == nil {
				if l := t.Size - p.GOP; l > 12 {
					t.Debug("resize", "gop", p.GOP, "before", t.Size, "after", t.Size-5)
					t.Reduce(5) //缩小缓冲环节省内存
				}
			}
		}
		if p.BufferTime > 0 {
			t.IDRingList.AddIDR(t.Ring)
			if t.HistoryRing == nil {
				t.HistoryRing = t.IDRing
			}
		} else {
			t.IDRing = t.Ring
		}
	}
	p.writeAV(t, data)
	return
}

func (p *Publisher) WriteAudio(data IAVFrame) (err error) {
	if !p.PubAudio || p.IsStopped() {
		return
	}
	t := p.AudioTrack
	if t == nil {
		t = &AVTrack{}
		t.Logger = p.Logger.With("track", "audio")
		t.Init(256)
		p.Lock()
		p.AudioTrack = t
		p.TransTrack[reflect.TypeOf(data)] = t
		if len(p.Subscribers) > 0 {
			p.State = PublisherStateSubscribed
		} else {
			p.State = PublisherStateTrackAdded
		}
		p.Unlock()
	}
	if t.ICodecCtx == nil {
		return data.DecodeConfig(t)
	}
	p.writeAV(t, data)
	return
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
	return
}

func (p *Publisher) GetAudioTrack(dataType reflect.Type) (t *AVTrack) {
	p.RLock()
	defer p.RUnlock()
	if t, ok := p.TransTrack[dataType]; ok {
		return t
	}
	return
}

func (p *Publisher) GetVideoTrack(dataType reflect.Type) (t *AVTrack) {
	p.RLock()
	defer p.RUnlock()
	if t, ok := p.TransTrack[dataType]; ok {
		return t
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
	p.TransTrack = old.TransTrack
	// for _, track := range p.TransTrack {
	// 	track.ICodecCtx = nil
	// }
}

func (p *Publisher) SnapShot() (ret *pb.StreamSnapShot) {
	ret = &pb.StreamSnapShot{}
	if p.VideoTrack != nil {
		p.VideoTrack.Ring.Do(func(v *AVFrame) {
			var snap pb.TrackSnapShot
			snap.CanRead = v.CanRead
			snap.Sequence = v.Sequence
			snap.Timestamp = uint32(v.Timestamp)
			snap.WriteTime = uint64(v.WriteTime.UnixNano())
			if v.Wrap != nil {
				snap.Wrap = &pb.Wrap{
					Timestamp: uint32(v.Wrap.GetTimestamp()),
					Size:      uint32(v.Wrap.GetSize()),
					Data:      v.Wrap.Print(),
				}
			}
			ret.VideoTrack = append(ret.VideoTrack, &snap)
		})
	}
	if p.AudioTrack != nil {
		p.AudioTrack.Ring.Do(func(v *AVFrame) {
			var snap pb.TrackSnapShot
			snap.CanRead = v.CanRead
			snap.Sequence = v.Sequence
			snap.Timestamp = uint32(v.Timestamp)
			snap.WriteTime = uint64(v.WriteTime.UnixNano())
			if v.Wrap != nil {
				snap.Wrap = &pb.Wrap{
					Timestamp: uint32(v.Wrap.GetTimestamp()),
					Size:      uint32(v.Wrap.GetSize()),
					Data:      v.Wrap.Print(),
				}
			}
			ret.AudioTrack = append(ret.AudioTrack, &snap)
		})
	}
	return
}