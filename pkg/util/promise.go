package util

import (
	"context"
	"errors"
)

type Promise[T any] struct {
	context.Context
	context.CancelCauseFunc
	Value T
	// timer *time.Timer
}

func NewPromise[T any](v T) *Promise[T] {
	p := &Promise[T]{Value: v}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(context.Background())
	// p.timer = time.AfterFunc(time.Second, func() {
	// 	p.CancelCauseFunc(ErrTimeout)
	// })
	return p
}

var ErrResolve = errors.New("promise resolved")
var ErrTimeout = errors.New("promise timeout")

func (p *Promise[T]) Resolve(v T) {
	p.Value = v
	p.CancelCauseFunc(ErrResolve)
}

func (p *Promise[T]) Fulfill(err error) {
	// p.timer.Stop()
	p.CancelCauseFunc(Conditoinal(err == nil, ErrResolve, err))
}
