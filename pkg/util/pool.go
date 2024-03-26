package util

import "net"

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

type RecyclebleMemory struct {
	IPool[[]byte]
	Data net.Buffers
}

func (r *RecyclebleMemory) Recycle() {
	if r.IPool != nil {
		for _, b := range r.Data {
			r.Put(b)
		}
	}
}
