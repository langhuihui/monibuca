package util

import (
	"context"
	"errors"
	"time"
)

type Promise struct {
	context.Context
	context.CancelCauseFunc
	timer *time.Timer
}

func NewPromise(ctx context.Context) *Promise {
	p := &Promise{}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(ctx)
	return p
}

func NewPromiseWithTimeout(ctx context.Context, timeout time.Duration) *Promise {
	p := &Promise{}
	p.Context, p.CancelCauseFunc = context.WithCancelCause(ctx)
	p.timer = time.AfterFunc(timeout, func() {
		p.CancelCauseFunc(ErrTimeout)
	})
	return p
}

var ErrResolve = errors.New("promise resolved")
var ErrTimeout = errors.New("promise timeout")

func (p *Promise) Resolve() {
	p.Fulfill(nil)
}

func (p *Promise) Reject(err error) {
	p.Fulfill(err)
}

func (p *Promise) Await() (err error) {
	<-p.Done()
	err = context.Cause(p.Context)
	if errors.Is(err, ErrResolve) {
		err = nil
	}
	return
}

func (p *Promise) Fulfill(err error) {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.CancelCauseFunc(Conditoinal(err == nil, ErrResolve, err))
}

func (p *Promise) IsPending() bool {
	return context.Cause(p.Context) == nil
}
