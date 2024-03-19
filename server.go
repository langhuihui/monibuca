package m7s

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"time"
	"unsafe"

	. "m7s.live/monibuca/v5/pkg"
)

type Server struct {
	context.Context         `json:"-" yaml:"-"`
	context.CancelCauseFunc `json:"-" yaml:"-"`
	Plugins                 []*Plugin
	EventBus                `json:"-" yaml:"-"`
	*slog.Logger
}

var DefaultServer = &Server{}

func NewServer() *Server {
	return &Server{}
}

func Run(ctx context.Context) {
	ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Second*10))
	DefaultServer.Run(ctx)
}

func (s *Server) Run(ctx context.Context) {
	s.Logger = slog.With("server", uintptr(unsafe.Pointer(s)))
	s.Context, s.CancelCauseFunc = context.WithCancelCause(ctx)
	s.EventBus = NewEventBus()
	s.Info("start")
	s.initPlugins()
	select {
	case <-s.Done():
		s.Warn("Server is done", "reason", context.Cause(s))
	case event := <-s.EventBus:
		for _, plugin := range s.Plugins {
			plugin.handler.OnEvent(event)
		}
	}
}

func (s *Server) Stop() {
	s.CancelCauseFunc(errors.New("stop"))
}

func (s *Server) initPlugins() {
	for _, plugin := range plugins {
		instance := reflect.New(plugin.Type).Interface().(IPlugin)
		p := reflect.ValueOf(instance).Elem().FieldByName("Plugin").Addr().Interface().(*Plugin)
		p.handler = instance
		if plugin.Disabled {
			continue
		}
		p.server = s
		s.Plugins = append(s.Plugins, p)
		instance.OnInit()
	}
}
