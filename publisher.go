package m7s

import (
	"reflect"
	"sync"
	"time"

	"m7s.live/m7s/v5/pb"
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
}

func (s *SpeedControl) speedControl(speed float64, ts time.Duration) {
	if speed != s.speed {
		s.speed = speed
		s.beginTime = time.Now()
		s.beginTimestamp = ts
	} else {
		elapsed := time.Since(s.beginTime)
		should := time.Duration(float64(ts) / speed)
		if needSleep := should - elapsed; needSleep > time.Second {
			time.Sleep(needSleep)
		}
	}
}

type AVTracks struct {
	*AVTrack
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
	SpeedControl
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
				err = ErrPublishTimeout
			}
			if !p.AudioTrack.IsEmpty() && !p.AudioTrack.LastValue.WriteTime.IsZero() && time.Since(p.AudioTrack.LastValue.WriteTime) > p.PublishTimeout {
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
	p.lastTs = frame.Timestamp
	if p.Enabled(p, TraceLevel) {
		codec := t.FourCC().String()
		size, data := frame.Wraps[0].GetSize(), frame.Wraps[0].String()
		p.Trace("write", "seq", frame.Sequence, "ts", frame.Timestamp, "codec", codec, "size", size, "data", data)
	}
	t.Step()
	p.speedControl(p.Publish.Speed, p.lastTs)
}

func (p *Publisher) WriteVideo(data IAVFrame) (err error) {
	if !p.PubVideo || p.IsStopped() {
		return
	}
	t := p.VideoTrack.AVTrack
	if t == nil {
		t = NewAVTrack(data, p.Logger.With("track", "video"), 256)
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
	isIDR, isSeq, raw, err := data.Parse(t)
	if err != nil || (isSeq && !isIDR) {
		p.Error("parse", "err", err)
		return err
	}
	t.Value.Raw = raw
	idr, hidr := t.IDRing.Load(), t.HistoryRing.Load()
	if isIDR {
		if idr != nil {
			p.GOP = int(t.Value.Sequence - idr.Value.Sequence)
			if hidr == nil {
				if l := t.Size - p.GOP; l > 12 && t.Size > 100 {
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
		t.Glow(5)
	}
	p.writeAV(t, data)
	if p.VideoTrack.Length > 1 && !p.VideoTrack.AVTrack.Ready.Pendding() {
		if t.LastValue.Raw == nil {
			t.LastValue.Raw, err = t.LastValue.Wraps[0].ToRaw(t.ICodecCtx)
			if err != nil {
				t.Error("to raw", "err", err)
				return err
			}
		}
		for i, track := range p.VideoTrack.Items[1:] {
			if track.ICodecCtx == nil {
				err = (reflect.New(track.FrameType.Elem()).Interface().(IAVFrame)).DecodeConfig(track, t.ICodecCtx)
				for rf := idr; rf != t.Ring; rf = rf.Next() {
					if i == 0 && rf.Value.Raw == nil {
						rf.Value.Raw, err = rf.Value.Wraps[0].ToRaw(t.ICodecCtx)
						if err != nil {
							t.Error("to raw", "err", err)
							return err
						}
					}
					p.writeSubAV(track, &rf.Value)
				}
				track.Ready.Fulfill(err)
				if err != nil {
					track.Error("DecodeConfig", "err", err)
					return
				}
			} else {
				p.writeSubAV(track, t.LastValue)
			}
		}
	}
	return
}

func (p *Publisher) writeSubAV(to *AVTrack, frame *AVFrame) (err error) {
	var toFrame IAVFrame
	if toFrame, err = to.CreateFrame(frame); err != nil {
		to.Error("from raw", "err", err)
		return
	}
	to.Value.Wraps = append(to.Value.Wraps, toFrame)
	to.Value.IDR = frame.IDR
	to.Value.Timestamp = frame.Timestamp
	if p.Enabled(p, TraceLevel) {
		p.Trace("write", "seq", to.Value.Sequence, "ts", to.Value.Timestamp, "codec", to.FourCC().String(), "size", toFrame.GetSize(), "data", toFrame.String())
	}
	to.Step()
	return
}

func (p *Publisher) WriteAudio(data IAVFrame) (err error) {
	if !p.PubAudio || p.IsStopped() {
		return
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
	if t.Ready.Pendding() {
		t.Ready.Fulfill(err)
		return
	}
	p.writeAV(t, data)
	return
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
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

func (p *Publisher) SnapShot() (ret *pb.StreamSnapShot) {
	ret = &pb.StreamSnapShot{}
	if !p.VideoTrack.IsEmpty() {
		p.VideoTrack.Ring.Do(func(v *AVFrame) {
			var snap pb.TrackSnapShot
			// snap.CanRead = v.CanRead
			snap.Sequence = v.Sequence
			snap.Timestamp = uint32(v.Timestamp)
			snap.WriteTime = uint64(v.WriteTime.UnixNano())
			// if v.Wrap != nil {
			// 	snap.Wrap = &pb.Wrap{
			// 		Timestamp: uint32(v.Wrap.GetTimestamp()),
			// 		Size:      uint32(v.Wrap.GetSize()),
			// 		Data:      v.Wrap.String(),
			// 	}
			// }
			ret.VideoTrack = append(ret.VideoTrack, &snap)
		})
	}
	if !p.AudioTrack.IsEmpty() {
		p.AudioTrack.Ring.Do(func(v *AVFrame) {
			var snap pb.TrackSnapShot
			// snap.CanRead = v.CanRead
			snap.Sequence = v.Sequence
			snap.Timestamp = uint32(v.Timestamp)
			snap.WriteTime = uint64(v.WriteTime.UnixNano())
			// if v.Wrap != nil {
			// 	snap.Wrap = &pb.Wrap{
			// 		Timestamp: uint32(v.Wrap.GetTimestamp()),
			// 		Size:      uint32(v.Wrap.GetSize()),
			// 		Data:      v.Wrap.String(),
			// 	}
			// }
			ret.AudioTrack = append(ret.AudioTrack, &snap)
		})
	}
	return
}
