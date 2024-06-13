package pkg

import (
	"context"
	"log/slog"
	"time"
)

const TraceLevel = slog.Level(-8)

type Unit struct {
	StartTime               time.Time
	*slog.Logger            `json:"-" yaml:"-"`
	context.Context         `json:"-" yaml:"-"`
	context.CancelCauseFunc `json:"-" yaml:"-"`
}

func (unit *Unit) Trace(msg string, fields ...any) {
	unit.Log(unit.Context, TraceLevel, msg, fields...)
}

func (unit *Unit) IsStopped() bool {
	return unit.StopReason() != nil
}

func (unit *Unit) StopReason() error {
	return context.Cause(unit.Context)
}

func (unit *Unit) Stop(err error) {
	unit.Info("stop", "reason", err.Error())
	unit.CancelCauseFunc(err)
}
