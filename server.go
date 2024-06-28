package m7s

import (
	"context"
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
	MergeConfigs  = []string{"Publish", "Subscribe", "HTTP", "PublicIP", "LogLevel", "EnableAuth"}
	ExecPath      = os.Args[0]
	ExecDir       = filepath.Dir(ExecPath)
	serverIndexG  atomic.Uint32
	DefaultServer = NewServer()
	serverMeta    = PluginMeta{
		Name:    "Global",
		Version: Version,
	}
	Servers           = make([]*Server, 10)
	defaultLogHandler = console.NewHandler(os.Stdout, &console.HandlerOptions{TimeFormat: "15:04:05.000000"})
)

type Server struct {
	pb.UnimplementedGlobalServer
	Plugin
	config.Engine
	ID              int
	eventChan       chan any
	Plugins         util.Collection[string, *Plugin]
	Streams         util.Collection[string, *Publisher]
	Pulls           util.Collection[string, *Puller]
	Pushs           util.Collection[string, *Pusher]
	Waiting         util.Collection[string, *Publisher]
	Subscribers     util.Collection[int, *Subscriber]
	LogHandler      MultiLogHandler
	pidG            int
	sidG            int
	apiList         []string
	grpcServer      *grpc.Server
	grpcClientConn  *grpc.ClientConn
	lastSummaryTime time.Time
	lastSummary     *pb.SummaryResponse
	OnAuthPubs      map[string]func(p *util.Promise[*Publisher])
	OnAuthSubs      map[string]func(p *util.Promise[*Subscriber])
}

func NewServer() (s *Server) {
	s = &Server{
		ID:        int(serverIndexG.Add(1)),
		eventChan: make(chan any, 10),
	}
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.handler = s
	s.server = s
	s.Meta = &serverMeta
	s.LogHandler.Add(defaultLogHandler)
	s.LogHandler.SetLevel(slog.LevelInfo)
	s.Logger = slog.New(&s.LogHandler).With("server", s.ID)
	s.OnAuthPubs = make(map[string]func(p *util.Promise[*Publisher]))
	s.OnAuthSubs = make(map[string]func(p *util.Promise[*Subscriber]))
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
		eventChan: make(chan any, 10),
	}
	server.Logger = s.Logger
	server.handler = s.handler
	server.server = s.server
	server.Meta = s.Meta
	server.config.HTTP.ListenAddrTLS = ":8443"
	server.config.HTTP.ListenAddr = ":8080"
	server.LogHandler = MultiLogHandler{}
	server.LogHandler.SetLevel(slog.LevelInfo)
	server.LogHandler.Add(defaultLogHandler)
	server.OnAuthPubs = s.OnAuthPubs
	server.OnAuthSubs = s.OnAuthSubs
	// server.Logger = slog.New(&server.LogHandler).With("server", s.ID)
	*s = server
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	s.StartTime = time.Now()
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
	s.Config.Parse(&s.config, "GLOBAL")
	s.Config.Parse(&s.Engine, "GLOBAL")
	if cg != nil {
		s.Config.ParseUserFile(cg["global"])
	}
	var lv slog.LevelVar
	lv.UnmarshalText([]byte(s.config.LogLevel))
	if s.config.LogLevel == "trace" {
		lv.Set(TraceLevel)
	}
	s.LogHandler.SetLevel(lv.Level())
	s.registerHandler(map[string]http.HandlerFunc{
		"/api/config/json/{name}":             s.api_Config_JSON_,
		"/api/stream/annexb/{streamPath...}":  s.api_Stream_AnnexB_,
		"/api/videotrack/sse/{streamPath...}": s.api_VideoTrack_SSE,
	})

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
	for publisher := range s.Streams.Range {
		publisher.Stop(err)
	}
	for subscriber := range s.Subscribers.Range {
		subscriber.Stop(err)
	}
	for p := range s.Plugins.Range {
		p.Stop(err)
	}
	httpConf.StopListen()
	return
}

type DoneChan = <-chan struct{}

func (s *Server) doneEventLoop(input chan DoneChan, output chan int) {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(input)}}
	for {
		switch chosen, rev, ok := reflect.Select(cases); chosen {
		case 0:
			if !ok {
				return
			}
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: rev})
		default:
			output <- chosen - 1
			cases = slices.Delete(cases, chosen, chosen+1)
		}
	}
}

func (s *Server) eventLoop() {
	pulse := time.NewTicker(s.PulseInterval)
	defer pulse.Stop()
	pubChan := make(chan DoneChan, 10)
	pubDoneChan := make(chan int, 10)
	subChan := make(chan DoneChan, 10)
	subDoneChan := make(chan int, 10)
	defer close(pubChan)
	defer close(subChan)
	go s.doneEventLoop(pubChan, pubDoneChan)
	go s.doneEventLoop(subChan, subDoneChan)
	for {
		select {
		case <-s.Done():
			return
		case <-pulse.C:
			for publisher := range s.Streams.Range {
				if err := publisher.checkTimeout(); err != nil {
					publisher.Stop(err)
				}
			}
			for publisher := range s.Waiting.Range {
				if publisher.Plugin != nil {
					if err := publisher.checkTimeout(); err != nil {
						publisher.Dispose(err)
						s.createWait(publisher.StreamPath)
					}
				}
				for sub := range publisher.SubscriberRange {
					select {
					case <-sub.TimeoutTimer.C:
						sub.Stop(ErrSubscribeTimeout)
					default:
					}
				}
			}
		case pubDone := <-pubDoneChan:
			s.onUnpublish(s.Streams.Items[pubDone])
		case subDone := <-subDoneChan:
			s.onUnsubscribe(s.Subscribers.Items[subDone])
		case event := <-s.eventChan:
			switch v := event.(type) {
			case *util.Promise[any]:
				switch vv := v.Value.(type) {
				case func():
					vv()
					v.Fulfill(nil)
					continue
				case func() error:
					v.Fulfill(vv())
					continue
				case *Publisher:
					err := s.OnPublish(vv)
					if v.Fulfill(err); err != nil {
						continue
					}
					event = vv
					pubChan <- vv.Done()
				case *Subscriber:
					err := s.OnSubscribe(vv)
					if v.Fulfill(err); err != nil {
						continue
					}
					subChan <- vv.Done()
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
						pubChan <- vv.Done()
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
						subChan <- vv.Done()
						s.Pushs.Add(vv)
						event = v.Value
					}
				}
			case slog.Handler:
				s.LogHandler.Add(v)
			}
			for plugin := range s.Plugins.Range {
				if plugin.Disabled {
					continue
				}
				plugin.onEvent(event)
			}
		}
	}
}

func (s *Server) onUnsubscribe(subscriber *Subscriber) {
	s.Subscribers.Remove(subscriber)
	s.Info("unsubscribe", "streamPath", subscriber.StreamPath, "reason", subscriber.StopReason())
	if subscriber.Closer != nil {
		subscriber.Close()
	}
	for pusher := range s.Pushs.Range {
		if &pusher.Subscriber == subscriber {
			s.Pushs.Remove(pusher)
			break
		}
	}
	if subscriber.Publisher != nil {
		subscriber.Publisher.RemoveSubscriber(subscriber)
	}
}

func (s *Server) onUnpublish(publisher *Publisher) {
	s.Streams.Remove(publisher)
	if publisher.Subscribers.Length > 0 {
		s.Waiting.Add(publisher)
	}
	s.Info("unpublish", "streamPath", publisher.StreamPath, "count", s.Streams.Length, "reason", publisher.StopReason())
	for subscriber := range publisher.SubscriberRange {
		waitCloseTimeout := publisher.WaitCloseTimeout
		if waitCloseTimeout == 0 {
			waitCloseTimeout = subscriber.WaitTimeout
		}
		subscriber.TimeoutTimer.Reset(waitCloseTimeout)
	}
	if publisher.Closer != nil {
		_ = publisher.Close()
	}
	s.Pulls.RemoveByKey(publisher.StreamPath)
}

func (s *Server) OnPublish(publisher *Publisher) error {
	if oldPublisher, ok := s.Streams.Get(publisher.StreamPath); ok {
		if publisher.KickExist {
			publisher.Warn("kick")
			oldPublisher.Stop(ErrKick)
			publisher.TakeOver(oldPublisher)
		} else {
			return ErrStreamExist
		}
	}
	s.Streams.Set(publisher)
	s.pidG++
	p := publisher.Plugin
	publisher.ID = s.pidG
	publisher.Logger = p.With("streamPath", publisher.StreamPath, "pubID", publisher.ID)
	publisher.TimeoutTimer = time.NewTimer(p.config.PublishTimeout)
	publisher.Start()
	if waiting, ok := s.Waiting.Get(publisher.StreamPath); ok {
		publisher.TakeOver(waiting)
		s.Waiting.Remove(waiting)
	}
	return nil
}

func (s *Server) createWait(streamPath string) *Publisher {
	newPublisher := &Publisher{}
	s.pidG++
	newPublisher.ID = s.pidG
	newPublisher.Logger = s.Logger.With("pubID", newPublisher.ID, "streamPath", streamPath)
	s.Info("createWait")
	newPublisher.StreamPath = streamPath
	s.Waiting.Set(newPublisher)
	return newPublisher
}

func (s *Server) OnSubscribe(subscriber *Subscriber) error {
	s.sidG++
	subscriber.ID = s.sidG
	subscriber.Logger = subscriber.Plugin.With("streamPath", subscriber.StreamPath, "subID", subscriber.ID)
	subscriber.TimeoutTimer = time.NewTimer(subscriber.Plugin.config.Subscribe.WaitTimeout)
	s.Subscribers.Add(subscriber)
	subscriber.Info("subscribe")
	if publisher, ok := s.Streams.Get(subscriber.StreamPath); ok {
		publisher.AddSubscriber(subscriber)
	} else if publisher, ok = s.Waiting.Get(subscriber.StreamPath); ok {
		publisher.AddSubscriber(subscriber)
	} else {
		s.createWait(subscriber.StreamPath).AddSubscriber(subscriber)
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		http.ServeFile(w, r, "favicon.ico")
		return
	}
	fmt.Fprintf(w, "visit:%s\nMonibuca Engine %s StartTime:%s\n", r.URL.Path, Version, s.StartTime)
	for plugin := range s.Plugins.Range {
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

func (s *Server) PostMessage(msg any) {
	s.eventChan <- msg
}
