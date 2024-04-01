package util

import (
	"net"
)

type Pool[T any] struct {
	pool []T
}

func (p *Pool[T]) Get() T {
	l := len(p.pool)
	if l == 0 {
		var t T
		return t
	}
	t := p.pool[l-1]
	p.pool = p.pool[:l-1]
	return t
}

func (p *Pool[T]) Clear() {
	p.pool = p.pool[:0]
}

func (p *Pool[T]) Put(t T) {
	p.pool = append(p.pool, t)
}

type IPool[T any] interface {
	Get() T
	Put(T)
	Clear()
}

type RecyclableMemory struct {
	IPool[[]byte]
	Data net.Buffers
}

func (r *RecyclableMemory) Recycle() {
	if r.IPool != nil {
		for _, b := range r.Data {
			r.Put(b)
		}
	}
}

type BytesPool struct {
	Pool[[]byte]
	ItemSize int
}

func (bp *BytesPool) GetN(size int) []byte {
	if size != bp.ItemSize {
		return make([]byte, size)
	}
	ret := bp.Pool.Get()
	if ret == nil {
		return make([]byte, size)
	}
	return ret
}

func (bp *BytesPool) Put(b []byte) {
	if cap(b) != bp.ItemSize {
		bp.ItemSize = cap(b)
		bp.Clear()
	}
	bp.Pool.Put(b)
}
