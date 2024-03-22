package pkg

import (
	"sync/atomic"

	"m7s.live/m7s/v5/pkg/util"
)

type emptyLocker struct{}

func (emptyLocker) Lock()   {}
func (emptyLocker) Unlock() {}

var EmptyLocker emptyLocker

// type IDataFrame interface {
// 	Reset()              // 重置数据,复用内存
// 	Ready()              // 标记为可读取
// 	ReaderEnter() int32  // 读取者数量+1
// 	ReaderLeave() int32  // 读取者数量-1
// 	StartWrite() bool    // 开始写入
// 	SetSequence(uint32)  // 设置序号
// 	GetSequence() uint32 // 获取序号
// 	ReaderCount() int32  // 读取者数量
// 	Discard() int32      // 如果写入时还有读取者没有离开则废弃该帧，剥离RingBuffer，防止并发读写
// 	IsDiscarded() bool   // 是否已废弃
// 	IsWriting() bool     // 是否正在写入
// 	Wait()               // 阻塞等待可读取
// 	Broadcast()          // 广播可读取
// }

type RingWriter struct {
	*util.Ring[AVFrame] `json:"-" yaml:"-"`
	ReaderCount   atomic.Int32 `json:"-" yaml:"-"`
	pool          *util.Ring[AVFrame]
	poolSize      int
	Size          int
	LastValue     *AVFrame
}

func (rb *RingWriter) Init(n int) *RingWriter {
	rb.Ring = util.NewRing[AVFrame](n)
	rb.Size = n
	rb.LastValue = &rb.Value
	return rb
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

func (rb *RingWriter) Recycle(r *util.Ring[AVFrame]) {
	rb.poolSize++
	r.Value.Reset()
	if rb.pool == nil {
		rb.pool = r
	} else {
		rb.pool.Link(r)
	}
}

func (rb *RingWriter) Reduce(size int) {
	r := rb.Unlink(size)
	if size > 1 {
		for p := r.Next(); p != r; {
			next := p.Next() //先保存下一个节点
			if p.Value.Discard() == 0 {
				rb.Recycle(p.Prev().Unlink(1))
			} else {
				// fmt.Println("Reduce", p.Value.ReaderCount())
			}
			p = next
		}
	}
	if r.Value.Discard() == 0 {
		rb.Recycle(r)
	}
	rb.Size -= size
	return
}

func (rb *RingWriter) Step() (normal bool) {
	rb.LastValue.Broadcast() // 防止订阅者还在等待
	rb.LastValue = &rb.Value
	nextSeq := rb.LastValue.GetSequence() + 1
	next := rb.Next()
	if normal = next.Value.StartWrite(); normal {
		next.Value.Reset()
		rb.Ring = next
	} else {
		rb.Reduce(1)         //抛弃还有订阅者的节点
		rb.Ring = rb.Glow(1) //补充一个新节点
		rb.Value.StartWrite()
	}
	rb.Value.SetSequence(nextSeq)
	rb.LastValue.Ready()
	return
}

func (rb *RingWriter) GetReaderCount() int32 {
	return rb.ReaderCount.Load()
}
