package m7s

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"google.golang.org/protobuf/proto"

	"m7s.live/v5/pkg/task"

	"m7s.live/v5/pkg/config"

	sysruntime "runtime"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/phsym/console-slog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"m7s.live/v5/pb"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/db"
	"m7s.live/v5/pkg/util"
)

var (
	Version      = "v5.0.0"
	MergeConfigs = [...]string{"Publish", "Subscribe", "HTTP", "PublicIP", "PublicIPv6", "LogLevel", "EnableAuth", "DB"}
	ExecPath     = os.Args[0]
	ExecDir      = filepath.Dir(ExecPath)
	serverMeta   = PluginMeta{
		Name:    "Global",
		Version: Version,
	}
	Servers           task.RootManager[uint32, *Server]
	defaultLogHandler = console.NewHandler(os.Stdout, &console.HandlerOptions{TimeFormat: "15:04:05.000000"})
)

type (
	ServerConfig struct {
		EnableSubEvent bool                     `default:"true" desc:"启用订阅事件,禁用可以提高性能"` //启用订阅事件,禁用可以提高性能
		SettingDir     string                   `default:".m7s" desc:""`
		FatalDir       string                   `default:"fatal" desc:""`
		PulseInterval  time.Duration            `default:"5s" desc:"心跳事件间隔"`    //心跳事件间隔
		DisableAll     bool                     `default:"false" desc:"禁用所有插件"` //禁用所有插件
		StreamAlias    map[config.Regexp]string `desc:"流别名"`
		Device         []*Device
	}
	WaitStream struct {
		StreamPath string
		SubscriberCollection
	}
	Server struct {
		pb.UnimplementedApiServer
		Plugin
		ServerConfig
		Plugins           util.Collection[string, *Plugin]
		Streams           task.Manager[string, *Publisher]
		AliasStreams      util.Collection[string, *AliasStream]
		Waiting           WaitManager
		Pulls             task.Manager[string, *PullJob]
		Pushs             task.Manager[string, *PushJob]
		Records           task.Manager[string, *RecordJob]
		Transforms        Transforms
		Devices           DeviceManager
		Subscribers       SubscriberCollection
		LogHandler        MultiLogHandler
		apiList           []string
		grpcServer        *grpc.Server
		grpcClientConn    *grpc.ClientConn
		lastSummaryTime   time.Time
		lastSummary       *pb.SummaryResponse
		conf              any
		configFileContent []byte
		prometheusDesc    prometheusDesc
	}
	CheckSubWaitTimeout struct {
		task.TickTask
		s *Server
	}
	GRPCServer struct {
		task.Task
		s       *Server
		tcpTask *config.ListenTCPWork
	}
	RawConfig = map[string]map[string]any
)

func (w *WaitStream) GetKey() string {
	return w.StreamPath
}

func NewServer(conf any) (s *Server) {
	s = &Server{
		conf: conf,
	}
	s.ID = task.GetNextTaskID()
	s.Meta = &serverMeta
	s.SetDescriptions(task.Description{
		"version":   Version,
		"goVersion": sysruntime.Version(),
		"os":        sysruntime.GOOS,
		"arch":      sysruntime.GOARCH,
		"cpus":      int32(sysruntime.NumCPU()),
	})
	//s.Transforms.PublishEvent = make(chan *Publisher, 10)
	s.prometheusDesc.init()
	return
}

func Run(ctx context.Context, conf any) (err error) {
	for err = ErrRestart; errors.Is(err, ErrRestart); err = Servers.Add(NewServer(conf), ctx).WaitStopped() {
	}
	return
}

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

var zipReader *zip.ReadCloser

func init() {
	Servers.Init()
	Servers.OnBeforeDispose(func() {
		time.AfterFunc(3*time.Second, exit)
	})
	Servers.OnDispose(exit)
	zipReader, _ = zip.OpenReader("admin.zip")
}

func (s *Server) GetKey() uint32 {
	return s.ID
}

type errLogger struct {
	*slog.Logger
}

func (l errLogger) Println(v ...interface{}) {
	l.Error("Exporter promhttp err: ", v...)
}

func (s *Server) Start() (err error) {
	s.Server = s
	s.handler = s
	httpConf, tcpConf := &s.config.HTTP, &s.config.TCP
	httpConf.ListenAddr = ":8080"
	tcpConf.ListenAddr = ":50051"
	s.LogHandler.SetLevel(slog.LevelDebug)
	s.LogHandler.Add(defaultLogHandler)
	s.Logger = slog.New(&s.LogHandler).With("server", s.ID)
	s.Waiting.Logger = s.Logger
	mux := runtime.NewServeMux(runtime.WithMarshalerOption("text/plain", &pb.TextPlain{}), runtime.WithForwardResponseOption(func(ctx context.Context, w http.ResponseWriter, m proto.Message) error {
		header := w.Header()
		header.Set("Access-Control-Allow-Credentials", "true")
		header.Set("Cross-Origin-Resource-Policy", "cross-origin")
		header.Set("Access-Control-Allow-Headers", "Content-Type,Access-Token")
		header.Set("Access-Control-Allow-Private-Network", "true")
		header.Set("Access-Control-Allow-Origin", "*")
		return nil
	}), runtime.WithRoutingErrorHandler(func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, r *http.Request, _ int) {
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
			s.Warn("read config file failed", "error", err.Error())
		} else {
			s.configFileContent = configYaml
		}
	case []byte:
		configYaml = v
	case RawConfig:
		cg = v
	}
	if configYaml != nil {
		if err = yaml.Unmarshal(configYaml, &cg); err != nil {
			s.Error("parsing yml", "error", err)
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
		s.AddDependTask(httpConf.CreateHTTPSWork(s.Logger))
	}
	if httpConf.ListenAddr != "" {
		s.AddDependTask(httpConf.CreateHTTPWork(s.Logger))
	}
	var grpcServer *GRPCServer
	if tcpConf.ListenAddr != "" {
		var opts []grpc.ServerOption
		s.grpcServer = grpc.NewServer(opts...)
		pb.RegisterApiServer(s.grpcServer, s)

		s.grpcClientConn, err = grpc.DialContext(s.Context, tcpConf.ListenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			s.Error("failed to dial", "error", err)
			return
		}
		if err = pb.RegisterApiHandler(s.Context, mux, s.grpcClientConn); err != nil {
			s.Error("register handler faild", "error", err)
			return
		}
		grpcServer = &GRPCServer{s: s, tcpTask: tcpConf.CreateTCPWork(s.Logger, nil)}
		if err = s.AddTask(grpcServer.tcpTask).WaitStarted(); err != nil {
			s.Error("failed to listen", "error", err)
			return
		}
	}
	s.AddTask(&s.Records)
	s.AddTask(&s.Streams)
	s.AddTask(&s.Pulls)
	s.AddTask(&s.Pushs)
	s.AddTask(&s.Transforms)
	s.AddTask(&s.Devices)
	promReg := prometheus.NewPedanticRegistry()
	promReg.MustRegister(s)
	for _, plugin := range plugins {
		p := plugin.Init(s, cg[strings.ToLower(plugin.Name)])
		if !p.Disabled {
			if collector, ok := p.handler.(prometheus.Collector); ok {
				promReg.MustRegister(collector)
			}
		}
	}
	promhttpHandler := promhttp.HandlerFor(prometheus.Gatherers{
		prometheus.DefaultGatherer,
		promReg,
	},
		promhttp.HandlerOpts{
			ErrorLog:      errLogger{s.Logger},
			ErrorHandling: promhttp.ContinueOnError,
		})
	s.handle("/api/metrics", promhttpHandler)
	if grpcServer != nil {
		s.AddTask(grpcServer, s.Logger)
	}
	s.Streams.OnStart(func() {
		s.Streams.AddTask(&CheckSubWaitTimeout{s: s})
	})
	// s.Transforms.AddTask(&TransformsPublishEvent{Transforms: &s.Transforms})
	s.Info("server started")
	s.Post(func() error {
		for plugin := range s.Plugins.Range {
			if plugin.Meta.Puller != nil {
				for streamPath, conf := range plugin.config.Pull {
					plugin.handler.Pull(streamPath, conf, nil)
				}
			}
			if plugin.Meta.Transformer != nil {
				for streamPath, conf := range plugin.config.Transform {
					transformer := plugin.Meta.Transformer()
					transformer.GetTransformJob().Init(transformer, plugin, streamPath, conf)
				}
			}
		}
		if s.DB != nil {
			s.DB.AutoMigrate(&Device{})
		}
		for _, d := range s.Device {
			if d.ID != 0 {
				d.server = s
				if d.Type == "" {
					u, err := url.Parse(d.URL)
					if err != nil {
						s.Error("parse pull url failed", "error", err)
						continue
					}
					switch u.Scheme {
					case "srt", "rtsp", "rtmp":
						d.Type = u.Scheme
					default:
						ext := filepath.Ext(u.Path)
						switch ext {
						case ".m3u8":
							d.Type = "hls"
						case ".flv":
							d.Type = "flv"
						case ".mp4":
							d.Type = "mp4"
						}
					}
				}
				if s.DB != nil {
					s.DB.Save(d)
				} else {
					s.Devices.Add(d, s.Logger.With("device", d.ID, "type", d.Type, "name", d.Name))
				}
			}
		}
		if s.DB != nil {
			var devices []*Device
			s.DB.Find(&devices)
			for _, d := range devices {
				d.server = s
				d.Logger = s.Logger.With("device", d.ID, "type", d.Type, "name", d.Name)
				d.ChangeStatus(DeviceStatusOffline)
				s.Devices.Add(d)
			}
		}
		return nil
	}, "serverStart")
	return
}

func (c *CheckSubWaitTimeout) GetTickInterval() time.Duration {
	return c.s.PulseInterval
}

func (c *CheckSubWaitTimeout) Tick(any) {
	percents, err := cpu.Percent(time.Second, false)
	if err == nil {
		for _, cpu := range percents {
			c.Info("tick", "cpu", cpu, "streams", c.s.Streams.Length, "subscribers", c.s.Subscribers.Length, "waits", c.s.Waiting.Length)
		}
	}
	c.s.Waiting.checkTimeout()
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

func (s *Server) OnSubscribe(streamPath string, args url.Values) {
	for plugin := range s.Plugins.Range {
		plugin.OnSubscribe(streamPath, args)
	}
	for device := range s.Devices.Range {
		if device.Status == DeviceStatusOnline && device.GetStreamPath() == streamPath && !device.PullOnStart {
			device.Handler.Pull()
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if zipReader != nil {
		http.ServeFileFS(w, r, zipReader, strings.TrimPrefix(r.URL.Path, "/admin"))
		return
	}
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
