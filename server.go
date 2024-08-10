package m7s

import (
	"context"
	"fmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	myip "github.com/husanpao/ip"
	"github.com/phsym/console-slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"log/slog"
	"m7s.live/m7s/v5/pb"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/db"
	"m7s.live/m7s/v5/pkg/util"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	Version       = "v5.0.0"
	MergeConfigs  = []string{"Publish", "Subscribe", "HTTP", "PublicIP", "LogLevel", "EnableAuth", "DB"}
	ExecPath      = os.Args[0]
	ExecDir       = filepath.Dir(ExecPath)
	DefaultServer = NewServer()
	serverMeta    = PluginMeta{
		Name:    "Global",
		Version: Version,
	}
	Servers           util.Collection[uint32, *Server]
	globalTask        MarcoTask
	Routes            = map[string]string{}
	defaultLogHandler = console.NewHandler(os.Stdout, &console.HandlerOptions{TimeFormat: "15:04:05.000000"})
)

type ServerConfig struct {
	EnableSubEvent bool          `default:"true" desc:"启用订阅事件,禁用可以提高性能"` //启用订阅事件,禁用可以提高性能
	SettingDir     string        `default:".m7s" desc:""`
	EventBusSize   int           `default:"10" desc:"事件总线大小"`    //事件总线大小
	PulseInterval  time.Duration `default:"5s" desc:"心跳事件间隔"`    //心跳事件间隔
	DisableAll     bool          `default:"false" desc:"禁用所有插件"` //禁用所有插件
}

type Server struct {
	pb.UnimplementedGlobalServer
	Plugin
	ServerConfig
	Plugins                                    util.Collection[string, *Plugin]
	Streams, Waiting                           util.Collection[string, *Publisher]
	Pulls                                      util.Collection[string, *PullContext]
	Pushs                                      util.Collection[string, *PushContext]
	Records                                    util.Collection[string, *RecordContext]
	Subscribers                                SubscriberCollection
	LogHandler                                 MultiLogHandler
	apiList                                    []string
	grpcServer                                 *grpc.Server
	grpcClientConn                             *grpc.ClientConn
	tcplis                                     net.Listener
	lastSummaryTime                            time.Time
	lastSummary                                *pb.SummaryResponse
	streamTask, pullTask, pushTask, recordTask MarcoTask
	conf                                       any
}

func NewServer() (s *Server) {
	s = &Server{}
	s.ID = globalTask.GetID()
	s.Meta = &serverMeta
	return
}

func Run(ctx context.Context, conf any) error {
	return DefaultServer.Run(ctx, conf)
}

type rawconfig = map[string]map[string]any

func init() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	globalTask.InitKeepAlive(context.Background(), nil, nil, signalChan, func(os.Signal) {
		for _, meta := range plugins {
			if meta.OnExit != nil {
				meta.OnExit()
			}
		}
		if serverMeta.OnExit != nil {
			serverMeta.OnExit()
		}
		os.Exit(0)
	})
	for k, v := range myip.LocalAndInternalIPs() {
		Routes[k] = v
		fmt.Println(k, v)
		if lastdot := strings.LastIndex(k, "."); lastdot >= 0 {
			Routes[k[0:lastdot]] = k
		}
	}
}

func (s *Server) GetKey() uint32 {
	return s.ID
}

func (s *Server) Init(ctx context.Context, conf any) {
	s.conf = conf
	s.Server = s
	s.handler = s
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.config.TCP.ListenAddr = ":50051"
	s.LogHandler.SetLevel(slog.LevelInfo)
	s.LogHandler.Add(defaultLogHandler)
	s.MarcoTask.Init(ctx, slog.New(&s.LogHandler).With("Server", s.ID), s)
}

func (s *Server) Start() (err error) {
	httpConf, tcpConf := &s.config.HTTP, &s.config.TCP
	mux := runtime.NewServeMux(runtime.WithMarshalerOption("text/plain", &pb.TextPlain{}), runtime.WithRoutingErrorHandler(func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, r *http.Request, _ int) {
		httpConf.GetHttpMux().ServeHTTP(w, r)
	}))
	httpConf.SetMux(mux)
	var cg rawconfig
	var configYaml []byte
	switch v := s.conf.(type) {
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
	s.Config.Parse(&s.ServerConfig, "GLOBAL")
	if cg != nil {
		s.Config.ParseUserFile(cg["global"])
	}
	//s.eventChan = make(chan any, s.EventBusSize)
	s.LogHandler.SetLevel(ParseLevel(s.config.LogLevel))
	s.registerHandler(map[string]http.HandlerFunc{
		"/api/config/json/{name}":             s.api_Config_JSON_,
		"/api/stream/annexb/{streamPath...}":  s.api_Stream_AnnexB_,
		"/api/videotrack/sse/{streamPath...}": s.api_VideoTrack_SSE,
	})
	if s.config.DSN != "" {
		if factory, ok := db.Factory[s.config.DBType]; ok {
			s.DB, err = gorm.Open(factory(s.config.DSN), &gorm.Config{})
			if err != nil {
				s.Error("failed to connect database", "error", err, "dsn", s.config.DSN, "type", s.config.DBType)
				return
			}
		}
	}
	if httpConf.ListenAddrTLS != "" {
		s.Info("https listen at ", "addr", httpConf.ListenAddrTLS)
		go func(addr string) {
			if err = httpConf.ListenTLS(); err != http.ErrServerClosed {
				s.Stop(err)
			}
			s.Info("https stop listen at ", "addr", addr)
		}(httpConf.ListenAddrTLS)
	}
	if httpConf.ListenAddr != "" {
		s.Info("http listen at ", "addr", httpConf.ListenAddr)
		go func(addr string) {
			if err = httpConf.Listen(); err != http.ErrServerClosed {
				s.Stop(err)
			}
			s.Info("http stop listen at ", "addr", addr)
		}(httpConf.ListenAddr)
	}
	if tcpConf.ListenAddr != "" {
		var opts []grpc.ServerOption
		s.grpcServer = grpc.NewServer(opts...)
		pb.RegisterGlobalServer(s.grpcServer, s)

		s.grpcClientConn, err = grpc.DialContext(s.Context, tcpConf.ListenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			s.Error("failed to dial", "error", err)
			return
		}
		if err = pb.RegisterGlobalHandler(s.Context, mux, s.grpcClientConn); err != nil {
			s.Error("register handler faild", "error", err)
			return
		}
		s.tcplis, err = net.Listen("tcp", tcpConf.ListenAddr)
		if err != nil {
			s.Error("failed to listen", "error", err)
			return
		}
	}
	for _, plugin := range plugins {
		if p := plugin.Init(s, cg[strings.ToLower(plugin.Name)]); !p.Disabled {
			s.WaitTaskAdded(p)
		}
	}
	if s.tcplis != nil {
		go func(addr string) {
			if err = s.grpcServer.Serve(s.tcplis); err != nil {
				s.Stop(err)
			} else {
				s.Info("grpc stop listen at ", "addr", addr)
			}
		}(tcpConf.ListenAddr)
	}
	s.streamTask.InitKeepAlive(s.Context, nil, nil, time.NewTicker(s.PulseInterval).C, func(time.Time) {
		for publisher := range s.Streams.Range {
			if err := publisher.checkTimeout(); err != nil {
				publisher.Stop(err)
			}
		}
		for publisher := range s.Waiting.Range {
			// TODO: ?
			//if publisher.Plugin != nil {
			//	if err := publisher.checkTimeout(); err != nil {
			//		publisher.Stop(err)
			//		s.createWait(publisher.StreamPath)
			//	}
			//}
			for sub := range publisher.SubscriberRange {
				select {
				case <-sub.TimeoutTimer.C:
					sub.Stop(ErrSubscribeTimeout)
				default:
				}
			}
		}
	})
	s.pullTask.InitKeepAlive(s.Context, nil, nil)
	s.pushTask.InitKeepAlive(s.Context, nil, nil)
	s.recordTask.InitKeepAlive(s.Context, nil, nil)
	s.AddTasks(&s.streamTask, &s.pullTask, &s.pushTask, &s.recordTask)
	Servers.Add(s)
	return
}

func (s *Server) CallOnStreamTask(callback func(*Task) error) {
	s.streamTask.Call(callback)
}

func (s *Server) Dispose() {
	Servers.Remove(s)
	_ = s.tcplis.Close()
	_ = s.grpcClientConn.Close()
	s.config.HTTP.StopListen()
}

func (s *Server) Run(ctx context.Context, conf any) (err error) {
	for {
		s.Init(ctx, conf)
		if err = globalTask.WaitTaskAdded(s); err != nil {
			return
		}
		if err = s.WaitStopped(); err != ErrRestart {
			return
		}
		var server Server
		server.ID = s.ID
		server.Meta = s.Meta
		server.DB = s.DB
		*s = server
	}
}

func (s *Server) createWait(streamPath string) *Publisher {
	newPublisher := &Publisher{}
	newPublisher.Logger = s.Logger.With("streamPath", streamPath)
	s.Info("createWait")
	newPublisher.StreamPath = streamPath
	s.Waiting.Set(newPublisher)
	return newPublisher
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
