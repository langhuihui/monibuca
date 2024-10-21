package pkg

import (
	"m7s.live/v5/pkg/util"
)

type RingReader struct {
	*util.Ring[AVFrame]
	locked bool
	Count  int // 读取的帧数
}

func (r *RingReader) StartRead(ring *util.Ring[AVFrame]) (err error) {
	r.Ring = ring
	if r.Value.discard {
		return ErrDiscard
	}
	r.Value.RLock()
	r.locked = true
	r.Count++
	return
}

func (r *RingReader) StopRead() {
	if r.locked {
		r.Value.RUnlock()
		r.locked = false
	}
}

func (r *RingReader) ReadNext() (err error) {
	return r.Read(r.Next())
}

func (r *RingReader) Read(ring *util.Ring[AVFrame]) (err error) {
	r.StopRead()
	return r.StartRead(ring)
}
