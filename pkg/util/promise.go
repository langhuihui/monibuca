package util

import "context"

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
