package m7s

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	myip "github.com/husanpao/ip"
	"github.com/phsym/console-slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"m7s.live/m7s/v5/pb"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/db"
	"m7s.live/m7s/v5/pkg/util"
)

var (
	Version      = "v5.0.0"
	MergeConfigs = []string{"Publish", "Subscribe", "HTTP", "PublicIP", "LogLevel", "EnableAuth", "DB"}
	ExecPath     = os.Args[0]
	ExecDir      = filepath.Dir(ExecPath)
	serverMeta   = PluginMeta{
		Name:    "Global",
		Version: Version,
	}
	Servers           util.Collection[uint32, *Server]
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

type WaitStream struct {
	*slog.Logger
	StreamPath string
	SubscriberCollection
	baseTs time.Duration
}

func (w *WaitStream) GetKey() string {
	return w.StreamPath
}

type Server struct {
	pb.UnimplementedGlobalServer
	Plugin
	ServerConfig
	Plugins                                    util.Collection[string, *Plugin]
	Streams                                    util.Collection[string, *Publisher]
	Waiting                                    util.Collection[string, *WaitStream]
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
	streamTask, pullTask, pushTask, recordTask util.MarcoLongTask
	conf                                       any
}

func NewServer(conf any) (s *Server) {
	s = &Server{
		conf: conf,
	}
	s.ID = util.GetNextTaskID()
	s.Meta = &serverMeta
	return
}

func Run(ctx context.Context, conf any) (err error) {
	for err = ErrRestart; errors.Is(err, ErrRestart); err = util.RootTask.AddTaskWithContext(ctx, NewServer(conf)).WaitStopped() {
	}
	return
}

func AddRootTask[T util.ITask](task T) T {
	util.RootTask.AddTask(task)
	return task
}

func AddRootTaskWithContext[T util.ITask](ctx context.Context, task T) T {
	util.RootTask.AddTaskWithContext(ctx, task)
	return task
}

type RawConfig = map[string]map[string]any

func init() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	util.RootTask.AddChan(signalChan, func(os.Signal) {
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

func (s *Server) Start() (err error) {
	s.Server = s
	s.handler = s
	s.config.HTTP.ListenAddrTLS = ":8443"
	s.config.HTTP.ListenAddr = ":8080"
	s.config.TCP.ListenAddr = ":50051"
	s.LogHandler.SetLevel(slog.LevelDebug)
	s.LogHandler.Add(defaultLogHandler)
	s.Logger = slog.New(&s.LogHandler).With("server", s.ID)
	httpConf, tcpConf := &s.config.HTTP, &s.config.TCP
	mux := runtime.NewServeMux(runtime.WithMarshalerOption("text/plain", &pb.TextPlain{}), runtime.WithRoutingErrorHandler(func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, r *http.Request, _ int) {
		httpConf.GetHttpMux().ServeHTTP(w, r)
	}))
	httpConf.SetMux(mux)
	var cg RawConfig
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
	case RawConfig:
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
	s.AddTask(&s.streamTask)
	s.AddTask(&s.pullTask)
	s.AddTask(&s.pushTask)
	s.AddTask(&s.recordTask)
	for _, plugin := range plugins {
		if p := plugin.Init(s, cg[strings.ToLower(plugin.Name)]); !p.Disabled {
			s.AddTask(p.handler).WaitStarted()
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
	s.streamTask.AddChan(time.NewTicker(s.PulseInterval).C, func(time.Time) {
		for waits := range s.Waiting.Range {
			for sub := range waits.Range {
				select {
				case <-sub.TimeoutTimer.C:
					sub.Stop(ErrSubscribeTimeout)
				default:
				}
			}
		}
	})
	Servers.Add(s)
	s.Info("server started")
	return
}

func (s *Server) CallOnStreamTask(callback func() error) {
	s.streamTask.Call(callback)
}

func (s *Server) AddPullTask(task *PullContext) *util.Task {
	return s.pullTask.AddTask(task)
}

func (s *Server) AddPushTask(task *PushContext) *util.Task {
	return s.pushTask.AddTask(task)
}

func (s *Server) Dispose() {
	Servers.Remove(s)
	_ = s.tcplis.Close()
	_ = s.grpcClientConn.Close()
	s.config.HTTP.StopListen()
	if s.DB != nil {
		db, err := s.DB.DB()
		if err == nil {
			err = db.Close()
		}
	}
}

func (s *Server) createWait(streamPath string) *WaitStream {
	newPublisher := &WaitStream{
		StreamPath: streamPath,
		Logger:     s.Logger.With("streamPath", streamPath),
	}
	s.Info("createWait")
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
