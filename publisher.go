package m7s

import (
	"reflect"
	"sync"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
)

type Publisher struct {
	PubSubBase
	config.Publish
	VideoTrack  *AVTrack
	AudioTrack  *AVTrack
	DataTrack   *DataTrack
	Subscribers map[*Subscriber]struct{}
	sync.RWMutex
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) (err error) {
	p.Lock()
	defer p.Unlock()
	p.Subscribers[subscriber] = struct{}{}
	if p.VideoTrack != nil {
		subscriber.VideoTrackReader = NewAVRingReader(p.VideoTrack)
	}
	if p.AudioTrack != nil {
		subscriber.AudioTrackReader = NewAVRingReader(p.AudioTrack)
	}
	return
}

func (p *Publisher) writeAV(t *AVTrack, data IAVFrame) (err error) {
	if t.ICodecCtx == nil {
		err = data.DecodeConfig(t)
	}
	t.Ring.Value.Wrap[0] = data
	if n := len(t.DataTypes); n > 1 {
		t.Ring.Value.Raw, err = data.ToRaw(t)
		if err != nil {
			return
		}
		if t.Ring.Value.Raw == nil {
			return
		}
		for i := 1; i < n; i++ {
			t.Ring.Value.Wrap[i] = reflect.New(t.DataTypes[i]).Interface().(IAVFrame)
			t.Ring.Value.Wrap[i].FromRaw(t, t.Ring.Value.Raw)
		}
	}
	t.Step()
	return
}

func (p *Publisher) WriteVideo(data IAVFrame) (err error) {
	if !p.PubVideo {
		return
	}
	t := p.VideoTrack
	if t == nil {
		t = &AVTrack{
			DataTypes: []reflect.Type{reflect.TypeOf(data)},
		}
		t.Logger = p.Logger.With("track", "video")
		t.Init(256)
		p.VideoTrack = t
		p.Lock()
		for sub := range p.Subscribers {
			sub.VideoTrackReader = NewAVRingReader(t)
		}
		p.Unlock()
	}
	return p.writeAV(t, data)
}

func (p *Publisher) WriteAudio(data IAVFrame) (err error) {
	if !p.PubAudio {
		return
	}
	t := p.AudioTrack
	if t == nil {
		t = &AVTrack{
			DataTypes: []reflect.Type{reflect.TypeOf(data)},
		}
		t.Logger = p.Logger.With("track", "audio")
		t.Init(256)
		p.AudioTrack = t
		p.Lock()
		for sub := range p.Subscribers {
			sub.AudioTrackReader = NewAVRingReader(t)
		}
		p.Unlock()
	}
	return p.writeAV(t, data)
}

func (p *Publisher) WriteData(data IDataFrame) (err error) {
	return
}
