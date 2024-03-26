package util

import (
	"context"
	"errors"
)

type Promise[T any] struct {
	context.Context
	context.CancelCauseFunc
	Value T
}

func NewPromise[T any](v T) *Promise[T] {
	p := &Promise[T]{Value: v}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(context.Background())
	return p
}

var ErrResolve = errors.New("promise resolved")

func (p *Promise[T]) Fulfill(err error) {
	p.CancelCauseFunc(Conditoinal(err == nil, ErrResolve, err))
}
