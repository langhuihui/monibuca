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
	TransTrack  map[reflect.Type]*AVTrack
	Subscribers map[*Subscriber]struct{}
	GOP         int
	sync.RWMutex
}

func (p *Publisher) AddSubscriber(subscriber *Subscriber) (err error) {
	p.Lock()
	defer p.Unlock()
	p.Subscribers[subscriber] = struct{}{}
	subscriber.Publisher = p
	return
}

func (p *Publisher) writeAV(t *AVTrack, data IAVFrame) (err error) {
	if t.ICodecCtx == nil {
		return data.DecodeConfig(t)
	}
	t.Ring.Value.Wrap = data
	// if n := len(t.DataTypes); n > 1 {
	// 	t.Ring.Value.Raw, err = data.ToRaw(t)
	// 	if err != nil {
	// 		return
	// 	}
	// 	if t.Ring.Value.Raw == nil {
	// 		return
	// 	}
	// 	for i := 1; i < n; i++ {
	// 		if len(t.Ring.Value.Wrap) <= i {
	// 			t.Ring.Value.Wrap = append(t.Ring.Value.Wrap, nil)
	// 		}
	// 		t.Ring.Value.Wrap[i] = reflect.New(t.DataTypes[i]).Interface().(IAVFrame)
	// 		t.Ring.Value.Wrap[i].FromRaw(t, t.Ring.Value.Raw)
	// 	}
	// }

	t.Step()
	return
}

func (p *Publisher) WriteVideo(data IAVFrame) (err error) {
	if !p.PubVideo {
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
		p.Unlock()
	}
	// if t.IDRing != nil {
	// 	p.GOP = int(t.Value.Sequence - t.IDRing.Value.Sequence)
	// 	if t.HistoryRing == nil {
	// 		t.Narrow(p.GOP)
	// 	}
	// }
	cur := t.Ring
	err = p.writeAV(t, data)
	if err == nil && data.IsIDR() {
		t.AddIDR(cur)
	}
	return
}

func (p *Publisher) WriteAudio(data IAVFrame) (err error) {
	if !p.PubAudio {
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
		p.Unlock()
	}
	return p.writeAV(t, data)
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
