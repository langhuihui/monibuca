package m7s

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"gopkg.in/yaml.v3"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var Version = "v5.0.0"
var MergeConfigs = []string{"Publish", "Subscribe", "HTTP"}
var (
	ExecPath = os.Args[0]
	ExecDir  = filepath.Dir(ExecPath)
)

type Server struct {
	Plugin
	config.Engine
	StartTime  time.Time
	Plugins    []*Plugin
	Publishers map[string]*Publisher
	Waiting    map[string]*Subscriber
}

var DefaultServer = NewServer()

func NewServer() *Server {
	return &Server{}
}

func Run(ctx context.Context, conf any) error {
	return DefaultServer.Run(ctx, conf)
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	s.Logger = slog.With("server", uintptr(unsafe.Pointer(s)))
	s.Context, s.CancelCauseFunc = context.WithCancelCause(ctx)
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.eventChan = make(chan any, 10)
	s.Info("start")

	var cg map[string]map[string]any
	var configYaml []byte
	switch v := conf.(type) {
	case string:
		if _, err = os.Stat(v); err != nil {
			v = filepath.Join(ExecDir, v)
		}
		if configYaml, err = os.ReadFile(v); err != nil {
			s.Warn("read config file error:", err.Error())
		}
	case []byte:
		configYaml = v
	case map[string]map[string]any:
		cg = v
	}
	if configYaml != nil {
		if err = yaml.Unmarshal(configYaml, &cg); err != nil {
			s.Error("parsing yml error:", err)
		}
	}
	s.Config.Parse(&s.Engine, "GLOBAL")
	if cg != nil {
		s.Config.ParseUserFile(cg["global"])
	}
	s.initPlugins(cg)
	pulse := time.NewTicker(s.PulseInterval)
	select {
	case <-s.Done():
		s.Warn("Server is done", "reason", context.Cause(s))
		pulse.Stop()
		return
	case <-pulse.C:
	case event := <-s.eventChan:
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
	return
}

func (s *Server) initPlugins(cg map[string]map[string]any) {
	for _, plugin := range plugins {
		plugin.Init(s, cg[strings.ToLower(plugin.Name)])
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
