package m7s

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/mcuadros/go-defaults"
	"github.com/phsym/console-slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
	"m7s.live/m7s/v5/pb"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

var (
	Version       = "v5.0.0"
	MergeConfigs  = []string{"Publish", "Subscribe", "HTTP"}
	ExecPath      = os.Args[0]
	ExecDir       = filepath.Dir(ExecPath)
	serverIndexG  atomic.Uint32
	DefaultServer = NewServer()
	serverMeta    = PluginMeta{
		Name:    "Global",
		Version: Version,
	}
	Servers = make([]*Server, 10)
)

type Server struct {
	pb.UnimplementedGlobalServer
	Plugin
	config.Engine
	ID             int
	eventChan      chan any
	Plugins        []*Plugin
	Streams        util.Collection[string, *Publisher]
	Pulls          util.Collection[string, *Puller]
	Pushs          util.Collection[string, *Pusher]
	Waiting        map[string][]*Subscriber
	Subscribers    util.Collection[int, *Subscriber]
	pidG           int
	sidG           int
	apiList        []string
	grpcServer     *grpc.Server
	grpcClientConn *grpc.ClientConn
}

func NewServer() (s *Server) {
	s = &Server{
		ID:        int(serverIndexG.Add(1)),
		Waiting:   make(map[string][]*Subscriber),
		eventChan: make(chan any, 10),
	}
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.Logger = slog.With("server", s.ID)
	s.handler = s
	s.server = s
	s.Meta = &serverMeta
	Servers[s.ID] = s
	return
}

func Run(ctx context.Context, conf any) error {
	return DefaultServer.Run(ctx, conf)
}

type rawconfig = map[string]map[string]any

func (s *Server) reset() {
	server := Server{
		ID:        s.ID,
		Waiting:   make(map[string][]*Subscriber),
		eventChan: make(chan any, 10),
	}
	server.Logger = s.Logger
	server.handler = s.handler
	server.server = s.server
	server.Meta = s.Meta
	server.config.HTTP.ListenAddrTLS = ":8443"
	server.config.HTTP.ListenAddr = ":8080"
	*s = server
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	for err = s.run(ctx, conf); err == ErrRestart; err = s.run(ctx, conf) {
		s.reset()
	}
	return
}

func (s *Server) run(ctx context.Context, conf any) (err error) {
	httpConf, tcpConf := &s.config.HTTP, &s.config.TCP
	mux := runtime.NewServeMux(runtime.WithMarshalerOption("text/plain", &pb.TextPlain{}), runtime.WithRoutingErrorHandler(runtime.RoutingErrorHandlerFunc(func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, r *http.Request, _ int) {
		httpConf.GetHttpMux().ServeHTTP(w, r)
	})))
	httpConf.SetMux(mux)
	s.Context, s.CancelCauseFunc = context.WithCancelCause(ctx)
	s.Info("start")
	var cg rawconfig
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
	case rawconfig:
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
	if s.LogLevel == "trace" {
		lv.Set(TraceLevel)
	}
	s.Logger = slog.New(
		console.NewHandler(os.Stdout, &console.HandlerOptions{Level: lv.Level()}),
	).With("server", s.ID)
	s.registerHandler()

	if httpConf.ListenAddrTLS != "" {
		s.Info("https listen at ", "addr", httpConf.ListenAddrTLS)
		go func(addr string) {
			if err := httpConf.ListenTLS(); err != http.ErrServerClosed {
				s.Stop(err)
			}
			s.Info("https stop listen at ", "addr", addr)
		}(httpConf.ListenAddrTLS)
	}
	if httpConf.ListenAddr != "" {
		s.Info("http listen at ", "addr", httpConf.ListenAddr)
		go func(addr string) {
			if err := httpConf.Listen(); err != http.ErrServerClosed {
				s.Stop(err)
			}
			s.Info("http stop listen at ", "addr", addr)
		}(httpConf.ListenAddr)
	}
	var tcplis net.Listener
	if tcpConf.ListenAddr != "" {
		var opts []grpc.ServerOption
		s.grpcServer = grpc.NewServer(opts...)
		pb.RegisterGlobalServer(s.grpcServer, s)

		s.grpcClientConn, err = grpc.DialContext(ctx, tcpConf.ListenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			s.Error("failed to dial", "error", err)
			return err
		}
		defer s.grpcClientConn.Close()
		if err = pb.RegisterGlobalHandler(ctx, mux, s.grpcClientConn); err != nil {
			s.Error("register handler faild", "error", err)
			return err
		}
		tcplis, err = net.Listen("tcp", tcpConf.ListenAddr)
		if err != nil {
			s.Error("failed to listen", "error", err)
			return err
		}
		defer tcplis.Close()
	}
	for _, plugin := range plugins {
		plugin.Init(s, cg[strings.ToLower(plugin.Name)])
	}
	if tcplis != nil {
		go func(addr string) {
			if err := s.grpcServer.Serve(tcplis); err != nil {
				s.Stop(err)
			}
			s.Info("grpc stop listen at ", "addr", addr)
		}(tcpConf.ListenAddr)
	}
	s.eventLoop()
	err = context.Cause(s)
	s.Warn("Server is done", "reason", err)
	for _, publisher := range s.Streams.Items {
		publisher.Stop(err)
	}
	for _, subscriber := range s.Subscribers.Items {
		subscriber.Stop(err)
	}
	for _, p := range s.Plugins {
		p.Stop(err)
	}
	httpConf.StopListen()
	return
}

func (s *Server) eventLoop() {
	pulse := time.NewTicker(s.PulseInterval)
	defer pulse.Stop()
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.Done())}, {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(pulse.C)}, {Dir: reflect.SelectRecv, Chan: reflect.ValueOf(s.eventChan)}}
	var pubCount, subCount int
	addPublisher := func(publisher *Publisher) {
		if nl := s.Streams.Length; nl > pubCount {
			pubCount = nl
			if subCount == 0 {
				cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(publisher.Done())})
			} else {
				cases = slices.Insert(cases, 3+pubCount, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(publisher.Done())})
			}
		}
	}
	addSubscriber := func(subscriber *Subscriber) {
		if nl := s.Subscribers.Length; nl > subCount {
			subCount = nl
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(subscriber.Done())})
		}
	}
	for {
		switch chosen, rev, _ := reflect.Select(cases); chosen {
		case 0:
			return
		case 1:
			for _, publisher := range s.Streams.Items {
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
			case *util.Promise[any]:
				switch vv := v.Value.(type) {
				case *Publisher:
					err := s.OnPublish(vv)
					if v.Fulfill(err); err != nil {
						continue
					}
					event = vv
					addPublisher(vv)
				case *Subscriber:
					err := s.OnSubscribe(vv)
					if v.Fulfill(err); err != nil {
						continue
					}
					addSubscriber(vv)
					if !s.EnableSubEvent {
						continue
					}
					event = v.Value
				case *Puller:
					if _, ok := s.Pulls.Get(vv.StreamPath); ok {
						v.Fulfill(ErrStreamExist)
						continue
					} else {
						err := s.OnPublish(&vv.Publisher)
						v.Fulfill(err)
						if err != nil {
							continue
						}
						s.Pulls.Add(vv)
						addPublisher(&vv.Publisher)
						event = v.Value
					}
				case *Pusher:
					if _, ok := s.Pushs.Get(vv.StreamPath); ok {
						v.Fulfill(ErrStreamExist)
						continue
					} else {
						err := s.OnSubscribe(&vv.Subscriber)
						v.Fulfill(err)
						if err != nil {
							continue
						}
						addSubscriber(&vv.Subscriber)
						s.Pushs.Add(vv)
						event = v.Value
					}
				case *pb.StreamSnapRequest:
					if pub, ok := s.Streams.Get(vv.StreamPath); ok {
						v.Resolve(pub)
					} else {
						v.Fulfill(ErrNotFound)
					}
					continue
				case *pb.StopSubscribeRequest:
					if subscriber, ok := s.Subscribers.Get(int(vv.Id)); ok {
						subscriber.Stop(errors.New("stop by api"))
						v.Fulfill(nil)
					} else {
						v.Fulfill(ErrNotFound)
					}
					continue
				case *pb.StreamListRequest:
					var streams []*pb.StreamInfo
					for _, publisher := range s.Streams.Items {
						streams = append(streams, &pb.StreamInfo{
							Path: publisher.StreamPath,
						})
					}
					v.Resolve(&pb.StreamListResponse{List: streams})
					continue
				}
			}
			for _, plugin := range s.Plugins {
				if plugin.Disabled {
					continue
				}
				plugin.onEvent(event)
			}
		default:
			if subStart, pubIndex := 3+pubCount, chosen-3; chosen < subStart {
				s.onUnpublish(s.Streams.Items[pubIndex])
				pubCount--
			} else {
				i := chosen - subStart
				s.onUnsubscribe(s.Subscribers.Items[i])
				subCount--
			}
			cases = slices.Delete(cases, chosen, chosen+1)
		}
	}
}

func (s *Server) onUnsubscribe(subscriber *Subscriber) {
	s.Subscribers.Remove(subscriber)
	s.Info("unsubscribe", "streamPath", subscriber.StreamPath)
	if subscriber.Closer != nil {
		subscriber.Close()
	}
	for _, pusher := range s.Pushs.Items {
		if &pusher.Subscriber == subscriber {
			s.Pushs.Remove(pusher)
			break
		}
	}
	if subscriber.Publisher != nil {
		subscriber.Publisher.RemoveSubscriber(subscriber)
	}
	if subscribers, ok := s.Waiting[subscriber.StreamPath]; ok {
		if index := slices.Index(subscribers, subscriber); index >= 0 {
			s.Waiting[subscriber.StreamPath] = slices.Delete(subscribers, index, index+1)
			if len(subscribers) == 1 {
				delete(s.Waiting, subscriber.StreamPath)
			}
		}
	}
}

func (s *Server) onUnpublish(publisher *Publisher) {
	s.Streams.Remove(publisher)
	s.Info("unpublish", "streamPath", publisher.StreamPath, "count", s.Streams.Length)
	for subscriber := range publisher.Subscribers {
		s.Waiting[publisher.StreamPath] = append(s.Waiting[publisher.StreamPath], subscriber)
		subscriber.TimeoutTimer.Reset(publisher.WaitCloseTimeout)
	}
	if publisher.Closer != nil {
		publisher.Close()
	}
	s.Pulls.RemoveByKey(publisher.StreamPath)
}

func (s *Server) OnPublish(publisher *Publisher) error {
	if oldPublisher, ok := s.Streams.Get(publisher.StreamPath); ok {
		if publisher.KickExist {
			publisher.Warn("kick")
			oldPublisher.Stop(ErrKick)
			publisher.TakeOver(oldPublisher)
			oldPublisher.Subscribers = nil
		} else {
			return ErrStreamExist
		}
	} else {
		publisher.Subscribers = make(map[*Subscriber]struct{})
	}
	s.Streams.Set(publisher)
	s.pidG++
	p := publisher.Plugin
	publisher.ID = s.pidG
	publisher.Logger = p.With("streamPath", publisher.StreamPath, "puber", publisher.ID)
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
	publisher.Info("publish")
	if subscribers, ok := s.Waiting[publisher.StreamPath]; ok {
		for i, subscriber := range subscribers {
			if i == 0 && subscriber.Publisher != nil {
				publisher.TakeOver(subscriber.Publisher)
			}
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
	s.Subscribers.Add(subscriber)
	subscriber.Info("subscribe")
	if publisher, ok := s.Streams.Get(subscriber.StreamPath); ok {
		return publisher.AddSubscriber(subscriber)
	} else {
		s.Waiting[subscriber.StreamPath] = append(s.Waiting[subscriber.StreamPath], subscriber)
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		http.ServeFile(w, r, "favicon.ico")
		return
	}
	fmt.Fprintf(w, "Monibuca Engine %s StartTime:%s\n", Version, s.StartTime)
	for _, plugin := range s.Plugins {
		fmt.Fprintf(w, "Plugin %s Version:%s\n", plugin.Meta.Name, plugin.Meta.Version)
	}
	for _, api := range s.apiList {
		fmt.Fprintf(w, "%s\n", api)
	}
}

func (s *Server) Call(arg any) (result any, err error) {
	promise := util.NewPromise(arg)
	s.eventChan <- promise
	<-promise.Done()
	result = promise.Value
	if err = context.Cause(promise.Context); err == util.ErrResolve {
		err = nil
	}
	return
}
