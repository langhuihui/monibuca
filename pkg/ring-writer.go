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
	if size < rb.poolSize {
		newItem = rb.pool.Unlink(size)
		rb.poolSize -= size
	} else if size == rb.poolSize {
		newItem = rb.pool
		rb.poolSize = 0
		rb.pool = nil
	} else {
		newItem = util.NewRing[AVFrame](size - rb.poolSize).Link(rb.pool)
		rb.poolSize = 0
		rb.pool = nil
	}
	rb.Link(newItem)
	rb.Size += size
	return
}

func (rb *RingWriter) recycle(r *util.Ring[AVFrame]) {
	rb.poolSize++
	r.Value.Reset()
	if rb.pool == nil {
		rb.pool = r
	} else {
		rb.pool.Link(r)
	}
}

func (rb *RingWriter) Reduce(size int) (r *util.Ring[AVFrame]) {
	r = rb.Unlink(size)
	for p := r.Next(); p != r; {
		next := p.Next() //先保存下一个节点
		if p.Value.TryLock() {
			rb.recycle(p)
			p.Value.Unlock()
		} else {
			p.Value.discard = true
			dr := p.Prev().Unlink(1)
			dr.Value.Reset()
		}
		p = next
	}
	rb.Size -= size
	return
}

func (rb *RingWriter) Step() (normal bool) {
	// rb.LastValue.Broadcast() // 防止订阅者还在等待
	rb.LastValue = &rb.Value
	nextSeq := rb.LastValue.Sequence + 1
	next := rb.Next()
	if normal = next.Value.StartWrite(); normal {
		next.Value.Reset()
		rb.Ring = next
	} else {
		rb.Reduce(1)         //抛弃还有订阅者的节点
		rb.Ring = rb.Glow(1) //补充一个新节点
		rb.Value.StartWrite()
	}
	rb.Value.Sequence = nextSeq
	rb.LastValue.Ready()
	return
}
