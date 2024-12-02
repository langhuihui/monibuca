package m7s

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"m7s.live/v5/pkg/task"

	"github.com/quic-go/quic-go"

	gatewayRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	myip "github.com/husanpao/ip"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/db"
	"m7s.live/v5/pkg/util"
)

type (
	DefaultYaml    string
	OnExitHandler  func()
	AuthPublisher  = func(*Publisher) *util.Promise
	AuthSubscriber = func(*Subscriber) *util.Promise

	PluginMeta struct {
		Name                string
		Version             string //插件版本
		Type                reflect.Type
		defaultYaml         DefaultYaml //默认配置
		ServiceDesc         *grpc.ServiceDesc
		RegisterGRPCHandler func(context.Context, *gatewayRuntime.ServeMux, *grpc.ClientConn) error
		Puller              Puller
		Pusher              Pusher
		Recorder            Recorder
		Transformer         Transformer
		OnExit              OnExitHandler
		OnAuthPub           AuthPublisher
		OnAuthSub           AuthSubscriber
	}

	iPlugin interface {
		nothing()
	}

	IPlugin interface {
		task.IJob
		OnInit() error
		OnStop()
		Pull(string, config.Pull, *config.Publish)
		Transform(*Publisher, config.Transform)
		OnPublish(*Publisher)
	}

	IRegisterHandler interface {
		RegisterHandler() map[string]http.HandlerFunc
	}

	IPullerPlugin interface {
		GetPullableList() []string
	}

	ITCPPlugin interface {
		OnTCPConnect(conn *net.TCPConn) task.ITask
	}

	IUDPPlugin interface {
		OnUDPConnect(conn *net.UDPConn) task.ITask
	}

	IQUICPlugin interface {
		OnQUICConnect(quic.Connection) task.ITask
	}

	IDevicePlugin interface {
		OnDeviceAdd(device *Device) any
	}
)

var plugins []PluginMeta

func (plugin *PluginMeta) Init(s *Server, userConfig map[string]any) (p *Plugin) {
	instance, ok := reflect.New(plugin.Type).Interface().(IPlugin)
	if !ok {
		panic("plugin must implement IPlugin")
	}
	p = reflect.ValueOf(instance).Elem().FieldByName("Plugin").Addr().Interface().(*Plugin)
	p.handler = instance
	p.Meta = plugin
	p.Server = s
	p.Logger = s.Logger.With("plugin", plugin.Name)
	upperName := strings.ToUpper(plugin.Name)
	if os.Getenv(upperName+"_ENABLE") == "false" {
		p.Disabled = true
		p.Warn("disabled by env")
		return
	}
	p.Config.Parse(p.GetCommonConf(), upperName)
	p.Config.Parse(instance, upperName)
	for _, fname := range MergeConfigs {
		if name := strings.ToLower(fname); p.Config.Has(name) {
			p.Config.Get(name).ParseGlobal(s.Config.Get(name))
		}
	}
	if plugin.defaultYaml != "" {
		var defaultConf map[string]any
		if err := yaml.Unmarshal([]byte(plugin.defaultYaml), &defaultConf); err != nil {
			p.Error("parsing default config", "error", err)
		} else {
			p.Config.ParseDefaultYaml(defaultConf)
		}
	}
	p.Config.ParseUserFile(userConfig)
	p.SetDescription("version", plugin.Version)
	if userConfig != nil {
		p.SetDescription("userConfig", userConfig)
	}
	finalConfig, _ := yaml.Marshal(p.Config.GetMap())
	p.Logger.Handler().(*MultiLogHandler).SetLevel(ParseLevel(p.config.LogLevel))
	p.Debug("config", "detail", string(finalConfig))
	if s.DisableAll {
		p.Disabled = true
	}
	if userConfig["enable"] == false {
		p.Disabled = true
	} else if userConfig["enable"] == true {
		p.Disabled = false
	}
	if p.Disabled {
		p.Warn("plugin disabled")
		return
	} else {
		p.assign()
	}
	p.Info("init", "version", plugin.Version)
	var err error
	if p.config.DSN == s.GetCommonConf().DSN {
		p.DB = s.DB
	} else if p.config.DSN != "" {
		if factory, ok := db.Factory[p.config.DBType]; ok {
			s.DB, err = gorm.Open(factory(p.config.DSN), &gorm.Config{})
			if err != nil {
				s.Error("failed to connect database", "error", err, "dsn", s.config.DSN, "type", s.config.DBType)
				p.Disabled = true
				return
			}
		}
	}
	s.AddTask(instance)
	return
}

// InstallPlugin 安装插件
func InstallPlugin[C iPlugin](options ...any) error {
	var c *C
	t := reflect.TypeOf(c).Elem()
	meta := PluginMeta{
		Name: strings.TrimSuffix(t.Name(), "Plugin"),
		Type: t,
	}

	_, pluginFilePath, _, _ := runtime.Caller(1)
	configDir := filepath.Dir(pluginFilePath)

	if _, after, found := strings.Cut(configDir, "@"); found {
		meta.Version = after
	} else {
		meta.Version = "dev"
	}
	for _, option := range options {
		switch v := option.(type) {
		case OnExitHandler:
			meta.OnExit = v
		case DefaultYaml:
			meta.defaultYaml = v
		case Puller:
			meta.Puller = v
		case Pusher:
			meta.Pusher = v
		case Recorder:
			meta.Recorder = v
		case Transformer:
			meta.Transformer = v
		case AuthPublisher:
			meta.OnAuthPub = v
		case AuthSubscriber:
			meta.OnAuthSub = v
		case *grpc.ServiceDesc:
			meta.ServiceDesc = v
		case func(context.Context, *gatewayRuntime.ServeMux, *grpc.ClientConn) error:
			meta.RegisterGRPCHandler = v
		}
	}
	plugins = append(plugins, meta)
	return nil
}

var _ IPlugin = (*Plugin)(nil)

type Plugin struct {
	task.Work
	Disabled bool
	Meta     *PluginMeta
	config   config.Common
	config.Config
	handler IPlugin
	Server  *Server
	DB      *gorm.DB
}

func (Plugin) nothing() {

}

func (p *Plugin) GetKey() string {
	return p.Meta.Name
}

func (p *Plugin) GetGlobalCommonConf() *config.Common {
	return p.Server.GetCommonConf()
}

func (p *Plugin) GetCommonConf() *config.Common {
	return &p.config
}

func (p *Plugin) GetHandler() IPlugin {
	return p.handler
}

func (p *Plugin) GetPublicIP(netcardIP string) string {
	if p.config.PublicIP != "" {
		return p.config.PublicIP
	}
	if publicIP := util.GetPublicIP(netcardIP); publicIP != netcardIP {
		return publicIP
	}
	localIp := myip.InternalIPv4()
	if publicIP := util.GetPublicIP(localIp); publicIP != localIp {
		return publicIP
	}
	return localIp
}

func (p *Plugin) settingPath() string {
	return filepath.Join(p.Server.SettingDir, strings.ToLower(p.Meta.Name)+".yaml")
}

func (p *Plugin) assign() {
	f, err := os.Open(p.settingPath())
	defer f.Close()
	if err == nil {
		var modifyConfig map[string]any
		err = yaml.NewDecoder(f).Decode(&modifyConfig)
		if err != nil {
			panic(err)
		}
		p.Config.ParseModifyFile(modifyConfig)
	}
	var handlerMap map[string]http.HandlerFunc
	if v, ok := p.handler.(IRegisterHandler); ok {
		handlerMap = v.RegisterHandler()
	}
	p.registerHandler(handlerMap)
}

func (p *Plugin) Start() (err error) {
	s := p.Server
	if p.Meta.ServiceDesc != nil && s.grpcServer != nil {
		s.grpcServer.RegisterService(p.Meta.ServiceDesc, p.handler)
		if p.Meta.RegisterGRPCHandler != nil {
			if err = p.Meta.RegisterGRPCHandler(p.Context, s.config.HTTP.GetGRPCMux(), s.grpcClientConn); err != nil {
				return
			} else {
				p.Info("grpc handler registered")
			}
		}
	}
	s.Plugins.Add(p)
	if err = p.listen(); err != nil {
		return
	}
	if err = p.handler.OnInit(); err != nil {
		return
	}
	return
}

func (p *Plugin) Dispose() {
	p.handler.OnStop()
	p.Server.Plugins.Remove(p)
}

func (p *Plugin) listen() (err error) {
	httpConf := &p.config.HTTP

	if httpConf.ListenAddrTLS != "" && (httpConf.ListenAddrTLS != p.Server.config.HTTP.ListenAddrTLS) {
		p.AddDependTask(httpConf.CreateHTTPSWork(p.Logger))
	}

	if httpConf.ListenAddr != "" && (httpConf.ListenAddr != p.Server.config.HTTP.ListenAddr) {
		p.AddDependTask(httpConf.CreateHTTPWork(p.Logger))
	}

	if tcphandler, ok := p.handler.(ITCPPlugin); ok {
		tcpConf := &p.config.TCP
		if tcpConf.ListenAddr != "" && tcpConf.AutoListen {
			if err = p.AddTask(tcpConf.CreateTCPWork(p.Logger, tcphandler.OnTCPConnect)).WaitStarted(); err != nil {
				return
			}
		}
		if tcpConf.ListenAddrTLS != "" && tcpConf.AutoListen {
			if err = p.AddTask(tcpConf.CreateTCPTLSWork(p.Logger, tcphandler.OnTCPConnect)).WaitStarted(); err != nil {
				return
			}
		}
	}

	if udpHandler, ok := p.handler.(IUDPPlugin); ok {
		udpConf := &p.config.UDP
		if udpConf.ListenAddr != "" && udpConf.AutoListen {
			if err = p.AddTask(udpConf.CreateUDPWork(p.Logger, udpHandler.OnUDPConnect)).WaitStarted(); err != nil {
				return
			}
		}
	}

	if quicHandler, ok := p.handler.(IQUICPlugin); ok {
		quicConf := &p.config.Quic
		if quicConf.ListenAddr != "" && quicConf.AutoListen {
			err = p.AddTask(quicConf.CreateQUICWork(p.Logger, quicHandler.OnQUICConnect)).WaitStarted()
		}
	}
	return
}

func (p *Plugin) OnInit() error {
	return nil
}

func (p *Plugin) OnStop() {

}

// TODO: use alias stream
func (p *Plugin) OnPublish(pub *Publisher) {
	onPublish := p.config.OnPub
	if p.Meta.Pusher != nil {
		for r, pushConf := range onPublish.Push {
			if pushConf.URL = r.Replace(pub.StreamPath, pushConf.URL); pushConf.URL != "" {
				p.Push(pub, pushConf)
			}
		}
	}
	if p.Meta.Recorder != nil {
		for r, recConf := range onPublish.Record {
			if recConf.FilePath = r.Replace(pub.StreamPath, recConf.FilePath); recConf.FilePath != "" {
				p.Record(pub, recConf, nil)
			}
		}
	}
	var owner = pub.Value(Owner)
	var isTransformer bool
	if owner != nil {
		_, isTransformer = owner.(ITransformer)
	}
	if p.Meta.Transformer != nil && !isTransformer {
		for r, tranConf := range onPublish.Transform {
			if group := r.FindStringSubmatch(pub.StreamPath); group != nil {
				for j, to := range tranConf.Output {
					for i, g := range group {
						to.Target = strings.ReplaceAll(to.Target, fmt.Sprintf("$%d", i), g)
					}
					targetUrl, err := url.Parse(to.Target)
					if err == nil {
						to.StreamPath = strings.TrimPrefix(targetUrl.Path, "/")
					}
					tranConf.Output[j] = to
				}
				p.Transform(pub, tranConf)
			}
		}
	}
}
func (p *Plugin) OnSubscribe(streamPath string, args url.Values) {
	//	var avoidTrans bool
	//AVOID:
	//	for trans := range server.Transforms.Range {
	//		for _, output := range trans.Config.Output {
	//			if output.StreamPath == s.StreamPath {
	//				avoidTrans = true
	//				break AVOID
	//			}
	//		}
	//	}
	for reg, conf := range p.config.OnSub.Pull {
		if p.Meta.Puller != nil {
			conf.Args = config.HTTPValus(args)
			conf.URL = reg.Replace(streamPath, conf.URL)
			p.handler.Pull(streamPath, conf, nil)
		}
	}

	//if !avoidTrans {
	//	for reg, conf := range plugin.GetCommonConf().OnSub.Transform {
	//		if plugin.Meta.Transformer != nil {
	//			if reg.MatchString(s.StreamPath) {
	//				if group := reg.FindStringSubmatch(s.StreamPath); group != nil {
	//					for j, c := range conf.Output {
	//						for i, value := range group {
	//							c.Target = strings.Replace(c.Target, fmt.Sprintf("$%d", i), value, -1)
	//						}
	//						conf.Output[j] = c
	//					}
	//				}
	//				plugin.handler.Transform(s.StreamPath, conf)
	//			}
	//		}
	//	}
	//}
}
func (p *Plugin) PublishWithConfig(ctx context.Context, streamPath string, conf config.Publish) (publisher *Publisher, err error) {
	publisher = createPublisher(p, streamPath, conf)
	if p.config.EnableAuth {
		onAuthPub := p.Meta.OnAuthPub
		if onAuthPub == nil {
			onAuthPub = p.Server.Meta.OnAuthPub
		}
		if onAuthPub != nil {
			if err = onAuthPub(publisher).Await(); err != nil {
				p.Warn("auth failed", "error", err)
				return
			}
		}
	}
	err = p.Server.Streams.AddTask(publisher, ctx).WaitStarted()
	return
}

func (p *Plugin) Publish(ctx context.Context, streamPath string) (publisher *Publisher, err error) {
	return p.PublishWithConfig(ctx, streamPath, p.config.Publish)
}

func (p *Plugin) SubscribeWithConfig(ctx context.Context, streamPath string, conf config.Subscribe) (subscriber *Subscriber, err error) {
	subscriber = createSubscriber(p, streamPath, conf)
	if p.config.EnableAuth {
		onAuthSub := p.Meta.OnAuthSub
		if onAuthSub == nil {
			onAuthSub = p.Server.Meta.OnAuthSub
		}
		if onAuthSub != nil {
			if err = onAuthSub(subscriber).Await(); err != nil {
				p.Warn("auth failed", "error", err)
				return
			}
		}
	}
	err = p.Server.Streams.AddTask(subscriber, ctx).WaitStarted()
	if err == nil {
		select {
		case <-subscriber.waitPublishDone:
			err = subscriber.Publisher.WaitTrack()
		case <-subscriber.Done():
			err = subscriber.StopReason()
		}
	}
	return
}

func (p *Plugin) Subscribe(ctx context.Context, streamPath string) (subscriber *Subscriber, err error) {
	return p.SubscribeWithConfig(ctx, streamPath, p.config.Subscribe)
}

func (p *Plugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) {
	puller := p.Meta.Puller(conf)
	if puller == nil {
		return
	}
	puller.GetPullJob().Init(puller, p, streamPath, conf, pubConf)
}

func (p *Plugin) Push(pub *Publisher, conf config.Push) {
	pusher := p.Meta.Pusher()
	job := pusher.GetPushJob().Init(pusher, p, pub.StreamPath, conf)
	job.Depend(pub)
}

func (p *Plugin) Record(pub *Publisher, conf config.Record, subConf *config.Subscribe) {
	recorder := p.Meta.Recorder()
	job := recorder.GetRecordJob().Init(recorder, p, pub.StreamPath, conf, subConf)
	job.Depend(pub)
}

func (p *Plugin) Transform(pub *Publisher, conf config.Transform) {
	transformer := p.Meta.Transformer()
	job := transformer.GetTransformJob().Init(transformer, p, pub.StreamPath, conf)
	job.Depend(pub)
}

func (p *Plugin) registerHandler(handlers map[string]http.HandlerFunc) {
	t := reflect.TypeOf(p.handler)
	v := reflect.ValueOf(p.handler)
	// 注册http响应
	for i, j := 0, t.NumMethod(); i < j; i++ {
		name := t.Method(i).Name
		if name == "ServeHTTP" {
			continue
		}
		switch handler := v.Method(i).Interface().(type) {
		case func(http.ResponseWriter, *http.Request):
			patten := strings.ToLower(strings.ReplaceAll(name, "_", "/"))
			p.handle(patten, http.HandlerFunc(handler))
		}
	}
	for patten, handler := range handlers {
		p.handle(patten, handler)
	}
	if rootHandler, ok := p.handler.(http.Handler); ok {
		p.handle("/", rootHandler)
	}
}

func (p *Plugin) logHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		p.Debug("visit", "path", r.URL.String(), "remote", r.RemoteAddr)
		name := strings.ToLower(p.Meta.Name)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/"+name)
		handler.ServeHTTP(rw, r)
	})
}

func (p *Plugin) handle(pattern string, handler http.Handler) {
	if p == nil {
		return
	}
	last := pattern == "/"
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	handler = p.logHandler(handler)
	p.config.HTTP.Handle(pattern, handler, last)
	if p.Server != p.handler {
		pattern = "/" + strings.ToLower(p.Meta.Name) + pattern
		p.Debug("http handle added to Server", "pattern", pattern)
		p.Server.config.HTTP.Handle(pattern, handler, last)
	}
	p.Server.apiList = append(p.Server.apiList, pattern)
}

func (p *Plugin) SaveConfig() (err error) {
	return Servers.AddTask(&SaveConfig{Plugin: p}).WaitStopped()
}

type SaveConfig struct {
	task.Task
	Plugin *Plugin
	file   *os.File
}

func (s *SaveConfig) Start() (err error) {
	if s.Plugin.Modify == nil {
		err = os.Remove(s.Plugin.settingPath())
		if err == nil {
			err = task.ErrTaskComplete
		}
	}
	s.file, err = os.OpenFile(s.Plugin.settingPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	return
}

func (s *SaveConfig) Run() (err error) {
	return yaml.NewEncoder(s.file).Encode(s.Plugin.Modify)
}

func (s *SaveConfig) Dispose() {
	s.file.Close()
}
