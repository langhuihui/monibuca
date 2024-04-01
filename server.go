package m7s

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	eventChan   chan any
	Plugins     []*Plugin
	Streams     map[string]*Publisher
	Waiting     map[string][]*Subscriber
	Publishers  []*Publisher
	Subscribers []*Subscriber
	pidG        int
	sidG        int
}

var DefaultServer = NewServer()

func NewServer() *Server {
	return &Server{
		Streams:   make(map[string]*Publisher),
		Waiting:   make(map[string][]*Subscriber),
		eventChan: make(chan any, 10),
	}
}

func Run(ctx context.Context, conf any) error {
	return DefaultServer.Run(ctx, conf)
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	s.Logger = slog.With("server", serverIndexG.Add(1))
	s.Context, s.CancelCauseFunc = context.WithCancelCause(ctx)
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.handler = s
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
	s.registerHandler()
	if s.config.HTTP.ListenAddrTLS != "" {
		s.Info("https listen at ", "addr", s.config.HTTP.ListenAddrTLS)
		go func() {
			s.Stop(s.config.HTTP.ListenTLS())
		}()
	}
	if s.config.HTTP.ListenAddr != "" {
		s.Info("http listen at ", "addr", s.config.HTTP.ListenAddr)
		go func() {
			s.Stop(s.config.HTTP.Listen())
		}()
	}
	for _, plugin := range plugins {
		plugin.Init(s, cg[strings.ToLower(plugin.Name)])
	}
	s.eventLoop()
	s.Warn("Server is done", "reason", context.Cause(s))
	for _, publisher := range s.Publishers {
		publisher.Stop(nil)
	}
	for _, subscriber := range s.Subscribers {
		subscriber.Stop(nil)
	}
	for _, p := range s.Plugins {
		p.Stop(nil)
	}
	return
}

func (s *Server) eventLoop() {
	pulse := time.NewTicker(s.PulseInterval)
	defer pulse.Stop()
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.Done())}, {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pulse.C)}, {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.eventChan)}}
	var pubCount, subCount int
	for {
		switch chosen, rev, _ := reflect.Select(cases); chosen {
		case 0:
			return
		case 1:
			for _, publisher := range s.Streams {
				if err := publisher.checkTimeout(); err != nil {
					publisher.Stop(err)
				}
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
		case 2:
			event := rev.Interface()
			switch v := event.(type) {
			case *util.Promise[*Publisher]:
				v.Fulfill(s.OnPublish(v.Value))
				event = v.Value
				if nl := len(s.Publishers); nl > pubCount {
					pubCount = nl
					if subCount == 0 {
						cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(v.Value.Done())})
					} else {
						cases = slices.Insert(cases, 3+pubCount, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(v.Value.Done())})
					}
				}
			case *util.Promise[*Subscriber]:
				v.Fulfill(s.OnSubscribe(v.Value))
				if nl := len(s.Subscribers); nl > subCount {
					subCount = nl
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(v.Value.Done())})
				}
				if !s.EnableSubEvent {
					continue
				}
				event = v.Value
			}
			for _, plugin := range s.Plugins {
				if plugin.Disabled {
					continue
				}
				plugin.onEvent(event)
			}
		default:
			if subStart := 3 + pubCount; chosen < subStart {
				s.onUnpublish(s.Publishers[chosen-3])
				pubCount--
				s.Publishers = slices.Delete(s.Publishers, chosen-3, chosen-2)
			} else {
				i := chosen - subStart
				s.onUnsubscribe(s.Subscribers[i])
				subCount--
				s.Subscribers = slices.Delete(s.Subscribers, i, i+1)
			}
			cases = slices.Delete(cases, chosen, chosen+1)
		}
	}
}

func (s *Server) onUnsubscribe(subscriber *Subscriber) {
	if subscriber.Publisher != nil {
		subscriber.Publisher.RemoveSubscriber(subscriber)
	}
}

func (s *Server) onUnpublish(publisher *Publisher) {
	delete(s.Streams, publisher.StreamPath)
	for subscriber := range publisher.Subscribers {
		s.Waiting[publisher.StreamPath] = append(s.Waiting[publisher.StreamPath], subscriber)
		subscriber.TimeoutTimer.Reset(publisher.WaitCloseTimeout)
	}
}

func (s *Server) OnPublish(publisher *Publisher) error {
	if oldPublisher, ok := s.Streams[publisher.StreamPath]; ok {
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
	s.Streams[publisher.StreamPath] = publisher
	s.Publishers = append(s.Publishers, publisher)
	s.pidG++
	p := publisher.Plugin
	publisher.ID = s.pidG
	publisher.Logger = p.With("streamPath", publisher.StreamPath, "puber", publisher.ID)
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
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
	s.Subscribers = append(s.Subscribers, subscriber)
	subscriber.Info("subscribe")
	if publisher, ok := s.Streams[subscriber.StreamPath]; ok {
		return publisher.AddSubscriber(subscriber)
	} else {
		s.Waiting[subscriber.StreamPath] = append(s.Waiting[subscriber.StreamPath], subscriber)
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}
