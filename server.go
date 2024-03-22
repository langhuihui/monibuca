package m7s

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"slices"
	"time"
	"unsafe"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)
var Version = "v5.0.0"
type Server struct {
	StartTime time.Time
	context.Context
	context.CancelCauseFunc
	EventBus
	*slog.Logger
	Config     config.Engine
	Plugins    []*Plugin
	Publishers map[string]*Publisher
	Waiting    map[string]*Subscriber
}

var DefaultServer = &Server{}

func NewServer() *Server {
	return &Server{}
}

func Run(ctx context.Context) {
	DefaultServer.Run(ctx)
}

func (s *Server) Run(ctx context.Context) {
	s.Logger = slog.With("server", uintptr(unsafe.Pointer(s)))
	s.Context, s.CancelCauseFunc = context.WithCancelCause(ctx)
	s.EventBus = NewEventBus(10)
	s.Config.HTTP.ListenAddrTLS = ":8443"
	s.Config.HTTP.ListenAddr = ":8080"
	s.Info("start")
	s.initPlugins()
	pulse := time.NewTicker(s.Config.PulseInterval)
	select {
	case <-s.Done():
		s.Warn("Server is done", "reason", context.Cause(s))
		pulse.Stop()
		return
	case <-pulse.C:
	case event := <-s.EventBus:
		switch v := event.(type) {
		case util.Promise[*Publisher]:
			v.CancelCauseFunc(s.OnPublish(v.Value))
		case util.Promise[*Subscriber]:
			v.CancelCauseFunc(s.OnSubscribe(v.Value))
		}
		for _, plugin := range s.Plugins {
			if plugin.Disabled {
				continue
			}
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
		p.Meta = &plugin
		p.server = s
		p.Logger = s.Logger.With("plugin", plugin.Name)
		p.Context, p.CancelCauseFunc = context.WithCancelCause(s.Context)
		s.Plugins = append(s.Plugins, p)
		p.OnInit()
		instance.OnInit()
	}
}

func (s *Server) OnPublish(publisher *Publisher) error {
	if oldPublisher, ok := s.Publishers[publisher.StreamPath]; ok {
		if publisher.KickExist {
			oldPlugin := oldPublisher.Plugin
			publisher.Warn("kick")
			oldPlugin.handler.(IPublishPlugin).OnStopPublish(oldPublisher, ErrKick)
			if index := slices.Index(oldPlugin.Publishers, oldPublisher); index != -1 {
				oldPlugin.Publishers = slices.Delete(oldPlugin.Publishers, index, index+1)
			}
			publisher.VideoTrack = oldPublisher.VideoTrack
			publisher.AudioTrack = oldPublisher.AudioTrack
			publisher.DataTrack = oldPublisher.DataTrack
			publisher.Subscribers = oldPublisher.Subscribers
			oldPublisher.Subscribers = nil
		} else {
			return ErrStreamExist
		}
	} else {
		s.Publishers[publisher.StreamPath] = publisher
		publisher.Plugin.Info("publish", "streamPath", publisher.StreamPath)
		publisher.Plugin.Publishers = append(publisher.Plugin.Publishers, publisher)
	}
	if subscriber, ok := s.Waiting[publisher.StreamPath]; ok {
		delete(s.Waiting, publisher.StreamPath)
		publisher.AddSubscriber(subscriber)
	}
	return nil
}

func (s *Server) OnSubscribe(subscriber *Subscriber) error {
	if publisher, ok := s.Publishers[subscriber.StreamPath]; ok {
		return publisher.AddSubscriber(subscriber)
	} else {
		s.Waiting[subscriber.StreamPath] = subscriber
	}
	return nil
}
