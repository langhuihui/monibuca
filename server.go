package m7s

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v3"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var Version = "v5.0.0"
var (
	MergeConfigs = []string{"Publish", "Subscribe", "HTTP"}
	ExecPath     = os.Args[0]
	ExecDir      = filepath.Dir(ExecPath)
	serverIndexG atomic.Uint32
)

type Server struct {
	Plugin
	config.Engine
	StartTime  time.Time
	Plugins    []*Plugin
	Publishers map[string]*Publisher
	Waiting    map[string][]*Subscriber
	pidG       int
	sidG       int
}

var DefaultServer = NewServer()

func NewServer() *Server {
	return &Server{
		Publishers: make(map[string]*Publisher),
		Waiting:    make(map[string][]*Subscriber),
	}
}

func Run(ctx context.Context, conf any) error {
	return DefaultServer.Run(ctx, conf)
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	s.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})).With("server", serverIndexG.Add(1))
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
			s.Warn("read config file faild", "error", err.Error())
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
	defaults.SetDefaults(&s.Engine)
	defaults.SetDefaults(&s.config)
	s.Config.Parse(&s.config)
	s.Config.Parse(&s.Engine, "GLOBAL")
	if cg != nil {
		s.Config.ParseUserFile(cg["global"])
	}
	var lv slog.LevelVar
	lv.UnmarshalText([]byte(s.LogLevel))
	slog.SetLogLoggerLevel(lv.Level())
	s.initPlugins(cg)
	pulse := time.NewTicker(s.PulseInterval)
	for {
		select {
		case <-s.Done():
			s.Warn("Server is done", "reason", context.Cause(s))
			pulse.Stop()
			return
		case <-pulse.C:
			for _, publisher := range s.Publishers {
				publisher.checkTimeout()
			}
			for subscriber := range s.Waiting {
				for _, sub := range s.Waiting[subscriber] {
					select {
					case <-sub.TimeoutTimer.C:
						sub.Stop(ErrSubscribeTimeout)
					default:
					}
				}
			}
		case event := <-s.eventChan:
			switch v := event.(type) {
			case *util.Promise[*Publisher]:
				v.Fulfill(s.OnPublish(v.Value))
				event = v.Value
			case *util.Promise[*Subscriber]:
				v.Fulfill(s.OnSubscribe(v.Value))
				if !s.EnableSubEvent {
					continue
				}
				event = v.Value
			case UnpublishEvent:
				s.onUnpublish(v.Publisher)
			case UnsubscribeEvent:

			}
			for _, plugin := range s.Plugins {
				if plugin.Disabled {
					continue
				}
				plugin.handler.OnEvent(event)
			}
		}
	}
}

func (s *Server) initPlugins(cg map[string]map[string]any) {
	for _, plugin := range plugins {
		plugin.Init(s, cg[strings.ToLower(plugin.Name)])
	}
}

func (s *Server) onUnpublish(publisher *Publisher) {
	delete(s.Publishers, publisher.StreamPath)
	for subscriber := range publisher.Subscribers {
		s.Waiting[publisher.StreamPath] = append(s.Waiting[publisher.StreamPath], subscriber)
		subscriber.TimeoutTimer.Reset(publisher.WaitCloseTimeout)
	}
}

func (s *Server) OnPublish(publisher *Publisher) error {
	if oldPublisher, ok := s.Publishers[publisher.StreamPath]; ok {
		if publisher.KickExist {
			publisher.Warn("kick")
			oldPublisher.Stop(ErrKick)
			publisher.VideoTrack = oldPublisher.VideoTrack
			publisher.AudioTrack = oldPublisher.AudioTrack
			publisher.DataTrack = oldPublisher.DataTrack
			publisher.Subscribers = oldPublisher.Subscribers
			publisher.TransTrack = oldPublisher.TransTrack
			oldPublisher.Subscribers = nil
		} else {
			return ErrStreamExist
		}
	} else {
		publisher.Subscribers = make(map[*Subscriber]struct{})
		publisher.TransTrack = make(map[reflect.Type]*AVTrack)
	}
	s.Publishers[publisher.StreamPath] = publisher
	s.pidG++
	p := publisher.Plugin
	publisher.ID = s.pidG
	publisher.Logger = p.With("streamPath", publisher.StreamPath, "puber", publisher.ID)
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
	p.Publishers = append(p.Publishers, publisher)
	publisher.Info("publish")
	if subscribers, ok := s.Waiting[publisher.StreamPath]; ok {
		for _, subscriber := range subscribers {
			publisher.AddSubscriber(subscriber)
		}
		delete(s.Waiting, publisher.StreamPath)
	}
	return nil
}

func (s *Server) OnSubscribe(subscriber *Subscriber) error {
	s.sidG++
	subscriber.ID = s.sidG
	subscriber.Logger = subscriber.Plugin.With("streamPath", subscriber.StreamPath, "suber", subscriber.ID)
	subscriber.TimeoutTimer = time.NewTimer(subscriber.Plugin.config.Subscribe.WaitTimeout)
	subscriber.Info("subscribe")
	if publisher, ok := s.Publishers[subscriber.StreamPath]; ok {
		return publisher.AddSubscriber(subscriber)
	} else {
		s.Waiting[subscriber.StreamPath] = append(s.Waiting[subscriber.StreamPath], subscriber)
	}
	return nil
}
