package pkg

import (
	"m7s.live/m7s/v5/pkg/util"
)

type RingReader struct {
	*util.Ring[AVFrame]
	Count int // 读取的帧数
}

func (r *RingReader) StartRead(ring *util.Ring[AVFrame]) (err error) {
	r.Ring = ring
	if r.Value.IsDiscarded() {
		return ErrDiscard
	}
	if r.Value.IsWriting() {
		// t := time.Now()
		r.Value.Wait()
		// log.Info("wait", time.Since(t))
	}
	r.Count++
	r.Value.ReaderEnter()
	return
}

func (r *RingReader) TryRead() (f *AVFrame, err error) {
	if r.Count > 0 {
		preValue := &r.Value
		if preValue.IsDiscarded() {
			preValue.ReaderLeave()
			err = ErrDiscard
			return
		}
		if r.Next().Value.IsWriting() {
			return
		}
		defer preValue.ReaderLeave()
		r.Ring = r.Next()
	} else {
		if r.Value.IsWriting() {
			return
		}
	}
	if r.Value.IsDiscarded() {
		err = ErrDiscard
		return
	}
	r.Count++
	f = &r.Value
	r.Value.ReaderEnter()
	return
}

func (r *RingReader) ReadNext() (err error) {
	return r.Read(r.Next())
}

func (r *RingReader) Read(ring *util.Ring[AVFrame]) (err error) {
	preValue := &r.Value
	defer preValue.ReaderLeave()
	if preValue.IsDiscarded() {
		return ErrDiscard
	}
	return r.StartRead(ring)
}
