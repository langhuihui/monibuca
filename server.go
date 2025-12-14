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
	"sync"
	"time"

	"m7s.live/v5/pkg/storage"

	"gopkg.in/yaml.v3"

	"github.com/shirou/gopsutil/v4/cpu"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5/pkg/config"

	sysruntime "runtime"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/phsym/console-slog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
	"m7s.live/v5/pb"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/db"
	"m7s.live/v5/pkg/util"
)

var (
	Version      = "v5.0.0"
	MergeConfigs = [...]string{"Publish", "Subscribe", "HTTP", "PublicIP", "PublicIPv6", "LogLevel", "EnableAuth", "DB", "Hook"}
	ExecPath     = os.Args[0]
	ExecDir      = filepath.Dir(ExecPath)
	ServerMeta   = PluginMeta{
		Name:         "Global",
		Version:      Version,
		NewPullProxy: NewHTTPPullPorxy,
		NewPuller:    NewAnnexBPuller,
	}
	Servers           task.RootManager[uint32, *Server]
	defaultLogHandler = console.NewHandler(os.Stdout, &console.HandlerOptions{TimeFormat: "15:04:05.000000"})
)

type (
	ServerConfig struct {
		FatalDir      string                   `default:"fatal" desc:""`
		PulseInterval time.Duration            `default:"5s" desc:"心跳事件间隔"`                               //心跳事件间隔
		DisableAll    bool                     `default:"false" desc:"禁用所有插件"`                            //禁用所有插件
		Armed         bool                     `default:"false" desc:"布防状态,true=布防(启用录像),false=撤防(禁用录像)"` //布防状态
		StreamAlias   map[config.Regexp]string `desc:"流别名"`
		Location      map[config.Regexp]string `desc:"HTTP路由转发规则,key为正则表达式,value为目标地址"`
		PullProxy     []*PullProxyConfig
		PushProxy     []*PushProxyConfig
		Admin         struct {
			zipReader      *zip.ReadCloser
			zipLastModTime time.Time
			lastCheckTime  time.Time
			EnableLogin    bool   `default:"false" desc:"启用登录机制"` //启用登录机制
			FilePath       string `default:"admin.zip" desc:"管理员界面文件路径"`
			HomePage       string `default:"home" desc:"管理员界面首页"`
			Users          []struct {
				Username string `desc:"用户名"`
				Password string `desc:"密码"`
				Role     string `default:"user" desc:"角色,可选值:admin,user"`
			} `desc:"用户列表,仅在启用登录机制时生效"`
		} `desc:"管理员界面配置"`
		Storage map[string]any
	}
	WaitStream struct {
		StreamPath string
		SubscriberCollection
		Progress SubscriptionProgress
	}
	Step struct {
		Name        string
		Description string
		Error       string
		StartedAt   time.Time
		CompletedAt time.Time
	}
	SubscriptionProgress struct {
		Steps       []Step
		CurrentStep int
	}
	Server struct {
		pb.UnimplementedApiServer
		pb.UnimplementedAuthServer
		Plugin

		ServerConfig
		Plugins           util.Collection[string, *Plugin]
		Streams           util.Manager[string, *Publisher]
		AliasStreams      util.Collection[string, *AliasStream]
		Waiting           WaitManager
		Pulls             task.WorkCollection[string, *PullJob]
		Pushs             task.WorkCollection[string, *PushJob]
		Records           task.WorkCollection[string, *RecordJob]
		Transforms        TransformManager
		PullProxies       PullProxyManager
		PushProxies       PushProxyManager
		Subscribers       SubscriberCollection
		LogHandler        MultiLogHandler
		redirectAdvisor   RedirectAdvisor
		redirectOnce      sync.Once
		apiList           []string
		grpcServer        *grpc.Server
		grpcClientConn    *grpc.ClientConn
		lastSummaryTime   time.Time
		lastSummary       *pb.SummaryResponse
		conf              any
		configFilePath    string
		configFileContent []byte
		disabledPlugins   []*Plugin
		prometheusDesc    prometheusDesc
		Storage           storage.Storage
	}
	CheckSubWaitTimeout struct {
		task.TickTask
		s *Server
	}
	RawConfig = map[string]map[string]any
)

func (w *WaitStream) GetKey() string {
	return w.StreamPath
}

func NewServer(conf any) (s *Server) {
	s = &Server{
		conf:            conf,
		disabledPlugins: make([]*Plugin, 0),
	}
	s.ID = task.GetNextTaskID()
	s.Meta = &ServerMeta
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
	for err = ErrRestart; errors.Is(err, ErrRestart); err = Servers.AddTask(NewServer(conf), ctx).WaitStopped() {
	}
	return
}

func exit() {
	for _, meta := range plugins {
		if meta.OnExit != nil {
			meta.OnExit()
		}
	}
	if ServerMeta.OnExit != nil {
		ServerMeta.OnExit()
	}
	os.Exit(0)
}

var checkInterval = time.Second * 3 // 检查间隔为3秒

func init() {
	Servers.Init()
	Servers.OnStop(func() {
		time.AfterFunc(3*time.Second, exit)
	})
	Servers.OnDispose(exit)
}

func (s *Server) loadAdminZip() {
	if s.Admin.zipReader != nil {
		s.Admin.zipReader.Close()
		s.Admin.zipReader = nil
	}
	if info, err := os.Stat(s.Admin.FilePath); err == nil {
		s.Admin.zipLastModTime = info.ModTime()
		s.Admin.zipReader, _ = zip.OpenReader(s.Admin.FilePath)
	}
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
	if err = util.CreateShutdownScript(); err != nil {
		s.Error("create shutdown script error:", err)
	}
	s.Server = s
	s.handler = s
	httpConf, tcpConf := &s.config.HTTP, &s.config.TCP
	httpConf.ListenAddr = ":8080"
	tcpConf.ListenAddr = ":50051"
	s.LogHandler.SetLevel(slog.LevelDebug)
	s.LogHandler.Add(defaultLogHandler)
	s.Logger = slog.New(&s.LogHandler).With("server", s.ID)
	s.Waiting.Logger = s.Logger

	var httpMux http.Handler = httpConf.CreateHttpMux()
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption("text/plain", &pb.TextPlain{}),
		runtime.WithRoutingErrorHandler(func(_ context.Context, _ *runtime.ServeMux, _ runtime.Marshaler, w http.ResponseWriter, r *http.Request, _ int) {
			httpMux.ServeHTTP(w, r)
		}),
	)
	httpConf.SetMux(mux)

	var cg RawConfig
	var configYaml []byte
	switch v := s.conf.(type) {
	case string:
		if _, err = os.Stat(v); err != nil {
			v = filepath.Join(ExecDir, v)
		}
		s.configFilePath = v
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
	for key, value := range cg {
		if strings.Contains(key, "-") {
			key = strings.ReplaceAll(key, "-", "")
			cg[key] = value
		} else if strings.Contains(key, "_") {
			key = strings.ReplaceAll(key, "_", "")
			cg[key] = value
		}
	}
	s.Config.Parse(&s.config, "GLOBAL")
	s.Config.Parse(&s.ServerConfig, "GLOBAL")
	if cg != nil {
		s.Config.ParseUserFile(cg["global"])
	}
	s.LogHandler.SetLevel(ParseLevel(s.config.LogLevel))
	s.initStorage()
	err = debug.SetCrashOutput(util.InitFatalLog(s.FatalDir), debug.CrashOptions{})
	if err != nil {
		s.Error("SetCrashOutput", "error", err)
		return
	}

	s.registerHandler(map[string]http.HandlerFunc{
		"/api/config/json/{name}":             s.api_Config_JSON_,
		"/api/config/yaml/all":                s.api_Config_YAML_All,
		"/api/stream/annexb/{streamPath...}":  s.api_Stream_AnnexB_,
		"/api/videotrack/sse/{streamPath...}": s.api_VideoTrack_SSE,
		"/api/audiotrack/sse/{streamPath...}": s.api_AudioTrack_SSE,
		"/annexb/{streamPath...}":             s.annexB,
	})

	if s.config.DSN != "" {
		if factory, ok := db.Factory[s.config.DBType]; ok {
			s.DB, err = gorm.Open(factory(s.config.DSN), &gorm.Config{})
			if err != nil {
				s.Error("failed to connect database", "error", err, "dsn", s.config.DSN, "type", s.config.DBType)
				return
			}
			sqlDB, _ := s.DB.DB()
			sqlDB.SetMaxIdleConns(25)
			sqlDB.SetMaxOpenConns(100)
			sqlDB.SetConnMaxLifetime(5 * time.Minute)
			// Auto-migrate models
			if err = s.DB.AutoMigrate(&db.User{}, &PullProxyConfig{}, &PushProxyConfig{}, &StreamAliasDB{}, &AlarmInfo{}); err != nil {
				s.Error("failed to auto-migrate models", "error", err)
				return
			}
			// Create users from configuration if EnableLogin is true
			if s.ServerConfig.Admin.EnableLogin {
				for _, userConfig := range s.ServerConfig.Admin.Users {
					var user db.User
					// Check if user exists
					if err = s.DB.Where("username = ?", userConfig.Username).First(&user).Error; err != nil {
						// Create user if not exists
						user = db.User{
							Username: userConfig.Username,
							Password: userConfig.Password,
							Role:     userConfig.Role,
						}
						if err = s.DB.Create(&user).Error; err != nil {
							s.Error("failed to create user", "error", err, "username", userConfig.Username)
							continue
						}
						s.Info("created user from config", "username", userConfig.Username)
					} else {
						// Update existing user with config values
						user.Password = userConfig.Password
						user.Role = userConfig.Role
						if err = s.DB.Save(&user).Error; err != nil {
							s.Error("failed to update user", "error", err, "username", userConfig.Username)
							continue
						}
						s.Info("updated user from config", "username", userConfig.Username)
					}
				}
			}
			// Create default admin user if no users exist
			var count int64
			s.DB.Model(&db.User{}).Count(&count)
			if count == 0 {
				adminUser := &db.User{
					Username: "admin",
					Password: "admin",
					Role:     "admin",
				}
				if err = s.DB.Create(adminUser).Error; err != nil {
					s.Error("failed to create default admin user", "error", err)
					return
				}
			}
		}
	}

	if httpConf.ListenAddrTLS != "" {
		s.AddDependTask(CreateHTTPSWork(httpConf, s.Logger))
	}
	if httpConf.ListenAddr != "" {
		s.AddDependTask(CreateHTTPWork(httpConf, s.Logger))
	}

	var grpcServer *GRPCServer
	if tcpConf.ListenAddr != "" {
		var opts []grpc.ServerOption
		// Add the auth interceptor
		opts = append(opts, grpc.UnaryInterceptor(s.AuthInterceptor()))
		s.grpcServer = grpc.NewServer(opts...)
		pb.RegisterApiServer(s.grpcServer, s)
		pb.RegisterAuthServer(s.grpcServer, s)
		s.grpcClientConn, err = grpc.DialContext(s.Context, tcpConf.ListenAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			s.Error("failed to dial", "error", err)
			return
		}
		s.Using(s.grpcClientConn)
		if err = pb.RegisterApiHandler(s.Context, mux, s.grpcClientConn); err != nil {
			s.Error("register handler failed", "error", err)
			return
		}
		if err = pb.RegisterAuthHandler(s.Context, mux, s.grpcClientConn); err != nil {
			s.Error("register auth handler failed", "error", err)
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
	s.AddTask(&s.PullProxies)
	s.AddTask(&s.PushProxies)
	s.AddTask(&webHookQueueTask)
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
	if s.PulseInterval > 0 {
		s.Streams.OnStart(func() {
			s.Streams.AddTask(&CheckSubWaitTimeout{s: s})
		})
	}
	s.loadAdminZip()
	// s.Transforms.AddTask(&TransformsPublishEvent{Transforms: &s.Transforms})
	s.Info("server started")
	s.OnStart(func() {
		for streamPath, conf := range s.config.Pull {
			s.Pull(streamPath, conf, nil)
		}
		for plugin := range s.Plugins.Range {
			if plugin.Meta.NewPuller != nil {
				for streamPath, conf := range plugin.config.Pull {
					plugin.handler.Pull(streamPath, conf, nil)
				}
			}
			if plugin.Meta.NewTransformer != nil {
				for streamPath := range plugin.config.Transform {
					plugin.onSubscribe(streamPath, url.Values{}) //按需转换
					// transformer := plugin.Meta.Transformer()
					// transformer.GetTransformJob().Init(transformer, plugin, streamPath, conf)
				}
			}
		}
		if s.DB != nil {
			s.initPullProxies()
			s.initPushProxies()
			s.initStreamAlias()
		} else {
			s.initPullProxiesWithoutDB()
			s.initPushProxiesWithoutDB()
		}
	})
	if sender, webhook := s.getHookSender(config.HookOnSystemStart); sender != nil {
		alarmInfo := AlarmInfo{
			AlarmName: string(config.HookOnSystemStart),
			AlarmType: config.AlarmStartupRunning,
		}
		sender(webhook, alarmInfo)
	}
	return
}

func (s *Server) initPullProxies() {
	// 1. First read all pull proxies from database, excluding disabled ones
	var pullProxies []*PullProxyConfig
	s.DB.Where("status != ?", PullProxyStatusDisabled).Find(&pullProxies)

	// Create a map for quick lookup of existing proxies
	existingPullProxies := make(map[uint]*PullProxyConfig)
	for _, proxy := range pullProxies {
		existingPullProxies[proxy.ID] = proxy
		proxy.Status = PullProxyStatusOffline
		proxy.InitializeWithServer(s)
	}

	// 2. Process and override with config data
	for _, configProxy := range s.PullProxy {
		if configProxy.ID != 0 {
			configProxy.InitializeWithServer(s)
			// Update or insert into database
			s.DB.Save(configProxy)

			// Override existing proxy or add to list
			if existing, ok := existingPullProxies[configProxy.ID]; ok {
				// Update existing proxy with config values
				existing.URL = configProxy.URL
				existing.Type = configProxy.Type
				existing.Name = configProxy.Name
				existing.PullOnStart = configProxy.PullOnStart
			} else {
				pullProxies = append(pullProxies, configProxy)
			}
		}
	}

	// 3. Finally add all proxies to collections, excluding disabled ones
	for _, proxy := range pullProxies {
		if proxy.CheckInterval == 0 {
			proxy.CheckInterval = time.Second * 10
		}
		if proxy.PullOnStart {
			proxy.Pull.MaxRetry = -1
		}
		if proxy.Status != PullProxyStatusDisabled {
			s.createPullProxy(proxy)
		}
	}
}

func (s *Server) initPushProxies() {
	// 1. Read all push proxies from database
	var pushProxies []*PushProxyConfig
	s.DB.Find(&pushProxies)

	// Create a map for quick lookup of existing proxies
	existingPushProxies := make(map[uint]*PushProxyConfig)
	for _, proxy := range pushProxies {
		existingPushProxies[proxy.ID] = proxy
		proxy.Status = PushProxyStatusOffline
		proxy.InitializeWithServer(s)
	}

	// 2. Process and override with config data
	for _, configProxy := range s.PushProxy {
		if configProxy.ID != 0 {
			configProxy.InitializeWithServer(s)
			// Update or insert into database
			s.DB.Save(configProxy)

			// Override existing proxy or add to list
			if existing, ok := existingPushProxies[configProxy.ID]; ok {
				// Update existing proxy with config values
				existing.URL = configProxy.URL
				existing.Type = configProxy.Type
				existing.Name = configProxy.Name
				existing.PushOnStart = configProxy.PushOnStart
				existing.Audio = configProxy.Audio
			} else {
				pushProxies = append(pushProxies, configProxy)
			}
		}
	}

	// 3. Finally add all proxies to collections
	for _, proxy := range pushProxies {
		s.createPushProxy(proxy)
	}
}

func (s *Server) initPullProxiesWithoutDB() {
	// Process config proxies without database
	for _, proxy := range s.PullProxy {
		if proxy.ID != 0 {
			proxy.InitializeWithServer(s)
			s.createPullProxy(proxy)
		}
	}
}

func (s *Server) initPushProxiesWithoutDB() {
	// Process config proxies without database
	for _, proxy := range s.PushProxy {
		if proxy.ID != 0 {
			proxy.InitializeWithServer(s)
			s.createPushProxy(proxy)
		}
	}
}

func (c *CheckSubWaitTimeout) GetTickInterval() time.Duration {
	return c.s.PulseInterval
}

func (c *CheckSubWaitTimeout) Tick(any) {
	percents, err := cpu.Percent(time.Second, false)
	if err == nil {
		for _, cpu := range percents {
			c.Info("tick", "cpu", fmt.Sprintf("%.2f%%", cpu), "streams", c.s.Streams.Length, "subscribers", c.s.Subscribers.Length, "waits", c.s.Waiting.Length)
		}
	}
	c.s.Waiting.checkTimeout()
}

func (s *Server) CallOnStreamTask(callback func()) {
	s.Streams.Call(callback)
}

func (s *Server) Dispose() {
	if s.DB != nil {
		db, err := s.DB.DB()
		if err == nil {
			if cerr := db.Close(); cerr != nil {
				s.Error("close db error", "error", cerr)
			}
		}
	}
}

func (s *Server) GetPublisher(streamPath string) (publisher *Publisher, err error) {
	var ok bool
	publisher, ok = s.Streams.SafeGet(streamPath)
	if !ok {
		err = ErrNotFound
		return
	}
	return
}

func (s *Server) OnPublish(p *Publisher) {
	for plugin := range s.Plugins.Range {
		plugin.onPublish(p)
	}
	for pushProxy := range s.PushProxies.Range {
		conf := pushProxy.GetConfig()
		if conf.Status == PushProxyStatusOnline && pushProxy.GetStreamPath() == p.StreamPath && !conf.PushOnStart {
			pushProxy.Push()
		}
	}
}

func (s *Server) OnSubscribe(streamPath string, args url.Values) {
	for plugin := range s.Plugins.Range {
		plugin.onSubscribe(streamPath, args)
	}
	for pullProxy := range s.PullProxies.Range {
		conf := pullProxy.GetConfig()
		if conf.Status == PullProxyStatusOnline && pullProxy.GetStreamPath() == streamPath {
			pullProxy.Pull()
			if w, ok := s.Waiting.Get(streamPath); ok {
				pullProxy.GetPullJob().Progress = &w.Progress
			}
		}
	}
}

// initStorage 创建全局存储实例，失败时回落到本地存储（空配置）
func (s *Server) initStorage() {
	for t, conf := range s.ServerConfig.Storage {
		st, err := storage.CreateStorage(t, conf)
		if err == nil {
			s.Storage = st
			s.Info("global storage created", "type", t)
			return
		}
		s.Warn("create storage failed", "type", t, "err", err)
	}
	// 兜底：local 需要路径，这里用当前目录
	if st, err := storage.CreateStorage("local", "."); err == nil {
		s.Storage = st
		s.Info("fallback to local storage", "path", ".")
	} else {
		s.Error("fallback local storage failed", "err", err)
	}
}
