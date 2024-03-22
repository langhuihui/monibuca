package util

type Pool[T any] struct {
	pool []*T
}

func (p *Pool[T]) Get() *T {
	l := len(p.pool)
	if l == 0 {
		return new(T)
	}
	t := p.pool[l-1]
	p.pool = p.pool[:l-1]
	return t
}

func (p *Pool[T]) Put(t *T) {
	p.pool = append(p.pool, t)
}
