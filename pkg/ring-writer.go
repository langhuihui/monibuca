package pkg

import (
	"log/slog"
	"sync"
	"time"

	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

type RingWriter struct {
	*util.Ring[AVFrame]
	sync.RWMutex
	IDRingList  util.List[*util.Ring[AVFrame]] // 关键帧链表
	BufferRange util.Range[time.Duration]
	SizeRange   util.Range[int]
	pool        *util.Ring[AVFrame]
	poolSize    int
	reduceVol   int
	Size        int
	LastValue   *AVFrame
	SLogger     *slog.Logger
}

func NewRingWriter(sizeRange util.Range[int]) (rb *RingWriter) {
	rb = &RingWriter{
		Size:      sizeRange[0],
		Ring:      util.NewRing[AVFrame](sizeRange[0]),
		SizeRange: sizeRange,
	}
	rb.LastValue = &rb.Value
	rb.LastValue.StartWrite()
	rb.IDRingList.Init()
	return
}

func (rb *RingWriter) Resize(size int) {
	if size > 0 {
		rb.glow(size, "test")
	} else {
		rb.reduce(-size)
	}
}

func (rb *RingWriter) glow(size int, reason string) (newItem *util.Ring[AVFrame]) {
	//before, poolBefore := rb.Size, rb.poolSize
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
	//rb.SLogger.Debug("glow", "reason", reason, "size", fmt.Sprintf("%d -> %d", before, rb.Size), "pool", fmt.Sprintf("%d -> %d", poolBefore, rb.poolSize))
	return
}

func (rb *RingWriter) reduce(size int) {
	_, poolBefore := rb.Size, rb.poolSize
	r := rb.Unlink(size)
	rb.Size -= size
	for range size {
		if r.Value.StartWrite() {
			rb.poolSize++
			r.Value.Reset()
			r.Value.Unlock()
		} else {
			rb.SLogger.Debug("discard", "seq", r.Value.Sequence)
			r.Value.Discard()
			r = r.Prev()
			r.Unlink(1)
		}
		r = r.Next()
	}
	if poolBefore != rb.poolSize {
		if rb.pool == nil {
			rb.pool = r
		} else {
			rb.pool.Link(r)
		}
	}
	//rb.SLogger.Debug("reduce", "size", fmt.Sprintf("%d -> %d", before, rb.Size), "pool", fmt.Sprintf("%d -> %d", poolBefore, rb.poolSize))
}

func (rb *RingWriter) Dispose() {
	rb.SLogger.Debug("dispose")
	rb.Value.Ready()
}

func (rb *RingWriter) GetIDR() *util.Ring[AVFrame] {
	rb.RLock()
	defer rb.RUnlock()
	if latest := rb.IDRingList.Back(); latest != nil {
		return latest.Value
	}
	return nil
}

func (rb *RingWriter) GetOldestIDR() *util.Ring[AVFrame] {
	rb.RLock()
	defer rb.RUnlock()
	if latest := rb.IDRingList.Front(); latest != nil {
		return latest.Value
	}
	return nil
}

func (rb *RingWriter) GetHistoryIDR(bufTime time.Duration) *util.Ring[AVFrame] {
	rb.RLock()
	defer rb.RUnlock()
	for item := rb.IDRingList.Back(); item != nil; item = item.Prev() {
		if item.Value.Value.Timestamp-rb.LastValue.Timestamp > bufTime {
			return item.Value
		}
	}
	return nil
}

func (rb *RingWriter) durationFrom(from *util.Ring[AVFrame]) time.Duration {
	return rb.Value.Timestamp - from.Value.Timestamp
}

func (rb *RingWriter) CurrentBufferTime() time.Duration {
	return rb.BufferRange[1]
}

func (rb *RingWriter) PushIDR() {
	rb.Lock()
	rb.IDRingList.PushBack(rb.Ring)
	rb.Unlock()
}

func (rb *RingWriter) Step() (normal bool) {
	isIDR := rb.Value.IDR
	next := rb.Next()
	if isIDR {
		rb.SLogger.Log(nil, task.TraceLevel, "add idr")
		rb.PushIDR()
	}
	if rb.IDRingList.Len() > 0 {
		oldIDR := rb.IDRingList.Front()
		rb.BufferRange[1] = rb.durationFrom(oldIDR.Value)
		// do not remove only idr
		if next == rb.IDRingList.Back().Value {
			if rb.Size < rb.SizeRange[1] {
				rb.glow(5, "only idr")
				next = rb.Next()
			}
		} else if next == oldIDR.Value {
			if nextOld := oldIDR.Next(); nextOld != nil && rb.durationFrom(nextOld.Value) > rb.BufferRange[0] {
				rb.SLogger.Log(nil, task.TraceLevel, "remove old idr")
				rb.Lock()
				rb.IDRingList.Remove(oldIDR)
				rb.Unlock()
			} else {
				rb.glow(5, "not enough buffer")
				next = rb.Next()
			}
		} else if rb.BufferRange[1] > rb.BufferRange[0] {
			for tmpP, reduceCount := rb.Next(), 0; reduceCount < 5; reduceCount++ {
				if tmpP == oldIDR.Value {
					rb.reduceVol = 0
					break
				}
				if tmpP = tmpP.Next(); reduceCount == 4 {
					if rb.Size > rb.SizeRange[0]+5 {
						if rb.reduceVol++; rb.reduceVol > 50 {
							rb.reduce(5)
							next = rb.Next()
							rb.reduceVol = 0
						}
					} else {
						rb.reduceVol = 0
					}
				}
			}
		}
	}

	rb.LastValue = &rb.Value
	nextSeq := rb.LastValue.Sequence + 1
	if normal = next.Value.StartWrite(); normal {
		next.Value.Reset()
		rb.Ring = next
	} else {
		rb.reduce(1)                   //抛弃还有订阅者的节点
		rb.Ring = rb.glow(1, "refill") //补充一个新节点
		normal = rb.Value.StartWrite()
		if !normal {
			panic("RingWriter.Step")
		}
	}
	rb.Value.Sequence = nextSeq
	rb.LastValue.Ready()
	return
}
