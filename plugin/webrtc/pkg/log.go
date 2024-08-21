package webrtc

import (
	"fmt"
	"log/slog"
)

type LoggerTransform struct {
	Logger *slog.Logger
}

func (l *LoggerTransform) Trace(msg string) {
	l.Logger.Log(nil, -8, msg)
}

func (l *LoggerTransform) Tracef(format string, args ...interface{}) {
	l.Trace(fmt.Sprintf(format, args...))
}

func (l *LoggerTransform) Debug(msg string) {
	l.Logger.Debug(msg)
}

func (l *LoggerTransform) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

func (l *LoggerTransform) Info(msg string) {
	l.Logger.Info(msg)
}

func (l *LoggerTransform) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

func (l *LoggerTransform) Warn(msg string) {
	l.Logger.Warn(msg)
}

func (l *LoggerTransform) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

func (l *LoggerTransform) Error(msg string) {
	l.Logger.Error(msg)
}

func (l *LoggerTransform) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}
