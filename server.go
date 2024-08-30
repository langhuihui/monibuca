package m7s

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"m7s.live/m7s/v5/pkg/task"

	"m7s.live/m7s/v5/pkg/config"

	sysruntime "runtime"

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
	Servers           task.RootManager[uint32, *Server]
	Routes            = map[string]string{}
	defaultLogHandler = console.NewHandler(os.Stdout, &console.HandlerOptions{TimeFormat: "15:04:05.000000"})
)

type ServerConfig struct {
	EnableSubEvent bool          `default:"true" desc:"启用订阅事件,禁用可以提高性能"` //启用订阅事件,禁用可以提高性能
	SettingDir     string        `default:".m7s" desc:""`
	FatalDir       string        `default:"fatal" desc:""`
	EventBusSize   int           `default:"10" desc:"事件总线大小"`    //事件总线大小
	PulseInterval  time.Duration `default:"5s" desc:"心跳事件间隔"`    //心跳事件间隔
	DisableAll     bool          `default:"false" desc:"禁用所有插件"` //禁用所有插件
}

type WaitStream struct {
	*slog.Logger
	StreamPath string
	SubscriberCollection
	baseTsAudio, baseTsVideo time.Duration
}

func (w *WaitStream) GetKey() string {
	return w.StreamPath
}

type Server struct {
	pb.UnimplementedGlobalServer
	Plugin
	ServerConfig
	Plugins         util.Collection[string, *Plugin]
	Streams         task.Manager[string, *Publisher]
	Waiting         util.Collection[string, *WaitStream]
	Pulls           task.Manager[string, *PullJob]
	Pushs           task.Manager[string, *PushJob]
	Records         task.Manager[string, *RecordJob]
	Transforms      Transforms
	Subscribers     SubscriberCollection
	LogHandler      MultiLogHandler
	apiList         []string
	grpcServer      *grpc.Server
	grpcClientConn  *grpc.ClientConn
	lastSummaryTime time.Time
	lastSummary     *pb.SummaryResponse
	conf            any
}

func NewServer(conf any) (s *Server) {
	s = &Server{
		conf: conf,
	}
	s.ID = task.GetNextTaskID()
	s.Meta = &serverMeta
	s.Description = map[string]any{
		"version":   Version,
		"goVersion": sysruntime.Version(),
		"os":        sysruntime.GOOS,
		"arch":      sysruntime.GOARCH,
		"cpus":      int32(sysruntime.NumCPU()),
	}
	return
}

func Run(ctx context.Context, conf any) (err error) {
	for err = ErrRestart; errors.Is(err, ErrRestart); err = Servers.Add(NewServer(conf), ctx).WaitStopped() {
	}
	return
}

type RawConfig = map[string]map[string]any

func exit() {
	for _, meta := range plugins {
		if meta.OnExit != nil {
			meta.OnExit()
		}
	}
	if serverMeta.OnExit != nil {
		serverMeta.OnExit()
	}
	os.Exit(0)
}

func init() {
	Servers.Init()
	Servers.OnDispose(exit)
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
	//s.config.HTTP.ListenAddrTLS = ":8443"
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
	err = debug.SetCrashOutput(util.InitFatalLog(s.FatalDir), debug.CrashOptions{})
	if err != nil {
		s.Error("SetCrashOutput", "error", err)
		return
	}
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
		s.stopOnError(httpConf.CreateHTTPSWork(s.Logger))
	}
	if httpConf.ListenAddr != "" {
		s.stopOnError(httpConf.CreateHTTPWork(s.Logger))
	}
	var grpcServer *GRPCServer
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
		grpcServer = &GRPCServer{s: s, tcpTask: tcpConf.CreateTCPWork(s.Logger, nil)}
		if err = s.AddTask(grpcServer.tcpTask).WaitStarted(); err != nil {
			s.Error("failed to listen", "error", err)
			return
		}
	}
	s.AddTaskLazy(&s.Records)
	s.AddTaskLazy(&s.Streams)
	s.AddTaskLazy(&s.Pulls)
	s.AddTaskLazy(&s.Pushs)
	s.AddTaskLazy(&s.Transforms)
	for _, plugin := range plugins {
		plugin.Init(s, cg[strings.ToLower(plugin.Name)])
	}
	if grpcServer != nil {
		s.AddTask(grpcServer, s.Logger)
	}
	s.Streams.OnStart(func() {
		s.Streams.AddTask(&CheckSubWaitTimeout{s: s})
	})
	s.Info("server started")
	s.Post(func() error {
		for plugin := range s.Plugins.Range {
			if plugin.Meta.Puller != nil {
				for streamPath, conf := range plugin.config.Pull {
					plugin.handler.Pull(streamPath, conf)
				}
			}
		}
		return nil
	})
	return
}

type CheckSubWaitTimeout struct {
	task.TickTask
	s *Server
}

func (c *CheckSubWaitTimeout) GetTickInterval() time.Duration {
	return c.s.PulseInterval
}

func (c *CheckSubWaitTimeout) Tick(any) {
	for waits := range c.s.Waiting.Range {
		for sub := range waits.Range {
			select {
			case <-sub.TimeoutTimer.C:
				sub.Stop(ErrSubscribeTimeout)
			default:
			}
		}
	}
}

type GRPCServer struct {
	task.Task
	s       *Server
	tcpTask *config.ListenTCPWork
}

func (gRPC *GRPCServer) Dispose() {
	gRPC.s.Stop(gRPC.StopReason())
}

func (gRPC *GRPCServer) Go() (err error) {
	return gRPC.s.grpcServer.Serve(gRPC.tcpTask.Listener)
}

func (s *Server) CallOnStreamTask(callback func() error) {
	s.Streams.Call(callback)
}

func (s *Server) Dispose() {
	_ = s.grpcClientConn.Close()
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
	_, _ = fmt.Fprintf(w, "visit:%s\nMonibuca Engine %s StartTime:%s\n", r.URL.Path, Version, s.StartTime)
	for plugin := range s.Plugins.Range {
		_, _ = fmt.Fprintf(w, "Plugin %s Version:%s\n", plugin.Meta.Name, plugin.Meta.Version)
	}
	for _, api := range s.apiList {
		_, _ = fmt.Fprintf(w, "%s\n", api)
	}
}
