package pkg

import (
	"sync/atomic"

	"m7s.live/m7s/v5/pkg/util"
)

type RingWriter struct {
	*util.Ring[AVFrame]
	IDRingList  //最近的关键帧位置，首屏渲染
	ReaderCount atomic.Int32
	pool        *util.Ring[AVFrame]
	poolSize    int
	Size        int
	LastValue   *AVFrame
}

func NewRingWriter(n int) (rb *RingWriter) {
	rb = &RingWriter{
		Size: n,
		Ring: util.NewRing[AVFrame](n),
	}
	rb.LastValue = &rb.Value
	rb.LastValue.StartWrite()
	return
}

func (rb *RingWriter) Resize(size int) {
	if size > 0 {
		rb.Glow(size)
	} else {
		rb.Reduce(-size)
	}
}

func (rb *RingWriter) Glow(size int) (newItem *util.Ring[AVFrame]) {
	if newCount := size - rb.poolSize; newCount > 0 {
		newItem = util.NewRing[AVFrame](newCount).Link(rb.pool)
		rb.poolSize = 0
	} else {
		newItem = rb.pool.Unlink(size)
		rb.poolSize -= size
	}
	if rb.poolSize == 0 {
		rb.pool = nil
	}
	rb.Link(newItem)
	rb.Size += size
	return
}

func (rb *RingWriter) recycle(r *util.Ring[AVFrame]) {
	if rb.pool == nil {
		rb.pool = r
	} else {
		rb.pool.Link(r)
	}
}

func (rb *RingWriter) Reduce(size int) (r *util.Ring[AVFrame]) {
	r = rb.Unlink(size)
	for range size {
		if r.Value.TryLock() {
			rb.poolSize++
			r.Value.Reset()
			r.Value.Unlock()
		} else {
			r.Value.Discard()
			r = r.Prev()
			r.Unlink(1)
		}
		r = r.Next()
	}
	rb.recycle(r)
	rb.Size -= size
	return
}

func (rb *RingWriter) Dispose() {
	rb.Value.Ready()
}

func (rb *RingWriter) Step() (normal bool) {
	rb.LastValue = &rb.Value
	nextSeq := rb.LastValue.Sequence + 1
	next := rb.Next()
	if normal = next.Value.StartWrite(); normal {
		next.Value.Reset()
		rb.Ring = next
	} else {
		rb.Reduce(1)         //抛弃还有订阅者的节点
		rb.Ring = rb.Glow(1) //补充一个新节点
		normal = rb.Value.StartWrite()
		if !normal {
			panic("RingWriter.Step")
		}
	}
	rb.Value.Sequence = nextSeq
	rb.LastValue.Ready()
	return
}
