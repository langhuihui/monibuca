package util

import (
	"context"
	"errors"
	"time"
)

type Promise[T any] struct {
	context.Context
	context.CancelCauseFunc
	Value T
	timer *time.Timer
}

func NewPromise[T any](v T) *Promise[T] {
	p := &Promise[T]{Value: v}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(context.Background())
	return p
}
func NewPromiseWithTimeout[T any](v T, timeout time.Duration) *Promise[T] {
	p := &Promise[T]{Value: v}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(context.Background())
	p.timer = time.AfterFunc(timeout, func() {
		p.CancelCauseFunc(ErrTimeout)
	})
	return p
}

var ErrResolve = errors.New("promise resolved")
var ErrTimeout = errors.New("promise timeout")

func (p *Promise[T]) Resolve(v T) {
	p.Value = v
	p.CancelCauseFunc(ErrResolve)
}

func (p *Promise[T]) Await() (T, error) {
	<-p.Done()
	err := context.Cause(p.Context)
	if errors.Is(err, ErrResolve) {
		err = nil
	}
	return p.Value, err
}

func (p *Promise[T]) Fulfill(err error) {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.CancelCauseFunc(Conditoinal(err == nil, ErrResolve, err))
}

func (p *Promise[T]) IsPending() bool {
	return context.Cause(p.Context) == nil
}
