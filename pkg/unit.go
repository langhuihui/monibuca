package pkg

import (
	"context"
	"log/slog"
	"time"
)

const TraceLevel = slog.Level(-8)

type Unit[T any] struct {
	ID        T
	StartTime time.Time
	*slog.Logger
	context.Context
	context.CancelCauseFunc
}

func (unit *Unit[T]) Trace(msg string, fields ...any) {
	unit.Log(unit.Context, TraceLevel, msg, fields...)
}

func (unit *Unit[T]) IsStopped() bool {
	return unit.StopReason() != nil
}

func (unit *Unit[T]) StopReason() error {
	return context.Cause(unit.Context)
}

func (unit *Unit[T]) Stop(err error) {
	unit.Info("stop", "reason", err.Error())
	unit.CancelCauseFunc(err)
}
