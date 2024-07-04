package m7s

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	gatewayRuntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type DefaultYaml string

type PluginMeta struct {
	Name                string
	Version             string //插件版本
	Type                reflect.Type
	defaultYaml         DefaultYaml //默认配置
	ServiceDesc         *grpc.ServiceDesc
	RegisterGRPCHandler func(context.Context, *gatewayRuntime.ServeMux, *grpc.ClientConn) error
}

func (plugin *PluginMeta) Init(s *Server, userConfig map[string]any) {
	instance, ok := reflect.New(plugin.Type).Interface().(IPlugin)
	if !ok {
		panic("plugin must implement IPlugin")
	}
	p := reflect.ValueOf(instance).Elem().FieldByName("Plugin").Addr().Interface().(*Plugin)
	p.handler = instance
	p.Meta = plugin
	p.server = s
	p.Logger = s.Logger.With("plugin", plugin.Name)
	p.Context, p.CancelCauseFunc = context.WithCancelCause(s.Context)
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
	err := instance.OnInit()
	if err != nil {
		p.Error("init", "error", err)
		p.Stop(err)
		return
	}
	if plugin.ServiceDesc != nil && s.grpcServer != nil {
		s.grpcServer.RegisterService(plugin.ServiceDesc, instance)
		if plugin.RegisterGRPCHandler != nil {
			err = plugin.RegisterGRPCHandler(p.Context, s.config.HTTP.GetGRPCMux(), s.grpcClientConn)
			if err != nil {
				p.Error("init", "error", err)
				p.Stop(err)
			}
		}
	}
	s.Plugins.Add(p)
	p.Start()
}

type iPlugin interface {
	nothing()
}

type IPlugin interface {
	OnInit() error
	OnEvent(any)
}

type ITCPPlugin interface {
	OnTCPConnect(*net.TCPConn)
}

type IUDPPlugin interface {
	OnUDPConnect(*net.UDPConn)
}

var plugins []PluginMeta

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
		meta.Version = pluginFilePath
	}
	for _, option := range options {
		switch v := option.(type) {
		case DefaultYaml:
			meta.defaultYaml = v
		case *grpc.ServiceDesc:
			meta.ServiceDesc = v
		case func(context.Context, *gatewayRuntime.ServeMux, *grpc.ClientConn) error:
			meta.RegisterGRPCHandler = v
		}
	}
	plugins = append(plugins, meta)
	return nil
}

type Plugin struct {
	Unit
	Disabled bool
	Meta     *PluginMeta
	config   config.Common
	config.Config
	handler IPlugin
	server  *Server
}

func (Plugin) nothing() {

}

func (p *Plugin) GetKey() string {
	return p.Meta.Name
}

func (p *Plugin) GetGlobalCommonConf() *config.Common {
	return p.server.GetCommonConf()
}

func (p *Plugin) GetCommonConf() *config.Common {
	return &p.config
}

func (p *Plugin) settingPath() string {
	return filepath.Join(p.server.SettingDir, strings.ToLower(p.Meta.Name)+".yaml")
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
	if v, ok := p.handler.(interface {
		RegisterHandler() map[string]http.HandlerFunc
	}); ok {
		handlerMap = v.RegisterHandler()
	}
	p.registerHandler(handlerMap)
}

func (p *Plugin) Stop(err error) {
	p.Unit.Stop(err)
	p.config.HTTP.StopListen()
	p.config.TCP.StopListen()
}

func (p *Plugin) Start() {
	httpConf := &p.config.HTTP
	if httpConf.ListenAddrTLS != "" && (httpConf.ListenAddrTLS != p.server.config.HTTP.ListenAddrTLS) {
		p.Info("https listen at ", "addr", httpConf.ListenAddrTLS)
		go func() {
			p.Stop(httpConf.ListenTLS())
		}()
	}
	if httpConf.ListenAddr != "" && (httpConf.ListenAddr != p.server.config.HTTP.ListenAddr) {
		p.Info("http listen at ", "addr", httpConf.ListenAddr)
		go func() {
			p.Stop(httpConf.Listen())
		}()
	}
	tcpConf := &p.config.TCP
	tcphandler, ok := p.handler.(ITCPPlugin)
	if !ok {
		tcphandler = p
	}

	if tcpConf.ListenAddr != "" && tcpConf.AutoListen {
		p.Info("listen tcp", "addr", tcpConf.ListenAddr)
		go func() {
			err := tcpConf.Listen(tcphandler.OnTCPConnect)
			if err != nil {
				p.Error("listen tcp", "addr", tcpConf.ListenAddr, "error", err)
				p.Stop(err)
			}
		}()
	}
	if tcpConf.ListenAddrTLS != "" && tcpConf.AutoListen {
		p.Info("listen tcp tls", "addr", tcpConf.ListenAddrTLS)
		go func() {
			err := tcpConf.ListenTLS(tcphandler.OnTCPConnect)
			if err != nil {
				p.Error("listen tcp tls", "addr", tcpConf.ListenAddrTLS, "error", err)
				p.Stop(err)
			}
		}()
	}
}

func (p *Plugin) OnInit() error {
	return nil
}

func (p *Plugin) onEvent(event any) {
	switch v := event.(type) {
	case *Publisher:
		if h, ok := p.handler.(interface{ OnPublish(*Publisher) }); ok {
			h.OnPublish(v)
		}
	case *Puller:
		if h, ok := p.handler.(interface{ OnPull(*Puller) }); ok {
			h.OnPull(v)
		}
	}
	p.handler.OnEvent(event)
}

func (p *Plugin) OnEvent(event any) {

}

func (p *Plugin) OnTCPConnect(conn *net.TCPConn) {
	p.handler.OnEvent(conn)
}

func (p *Plugin) Publish(streamPath string, options ...any) (publisher *Publisher, err error) {
	publisher = &Publisher{Publish: p.config.Publish}
	if p.config.EnableAuth {
		if onAuthPub, ok := p.server.OnAuthPubs[p.Meta.Name]; ok {
			authPromise := util.NewPromise(publisher)
			onAuthPub(authPromise)
			if _, err = authPromise.Await(); err != nil {
				p.Warn("auth failed", "error", err)
				return
			}
		}
	}
	for _, option := range options {
		switch v := option.(type) {
		case func(*config.Publish):
			v(&publisher.Publish)
		}
	}
	publisher.Init(p, streamPath, &publisher.Publish, options...)
	_, err = p.server.Call(publisher)
	return
}

func (p *Plugin) Pull(streamPath string, url string, options ...any) (puller *Puller, err error) {
	puller = &Puller{Pull: p.config.Pull}
	puller.Client.Proxy = p.config.Pull.Proxy
	puller.Client.RemoteURL = url
	puller.Client.PubSubBase = &puller.PubSubBase
	puller.Publish = p.config.Publish
	puller.PublishTimeout = 0
	puller.StreamPath = streamPath
	var pullHandler PullHandler
	for _, option := range options {
		switch v := option.(type) {
		case PullHandler:
			pullHandler = v
		}
	}
	puller.Init(p, streamPath, &puller.Publish, options...)
	_, err = p.server.Call(puller)
	if err == nil && pullHandler != nil {
		err = puller.Start(pullHandler)
	}
	return
}

func (p *Plugin) Subscribe(streamPath string, options ...any) (subscriber *Subscriber, err error) {
	subscriber = &Subscriber{Subscribe: p.config.Subscribe}
	if p.config.EnableAuth {
		if onAuthSub, ok := p.server.OnAuthSubs[p.Meta.Name]; ok {
			authPromise := util.NewPromise(subscriber)
			onAuthSub(authPromise)
			if _, err = authPromise.Await(); err != nil {
				p.Warn("auth failed", "error", err)
				return
			}
		}
	}
	for _, option := range options {
		switch v := option.(type) {
		case func(*config.Subscribe):
			v(&subscriber.Subscribe)
		}
	}
	subscriber.Init(p, streamPath, &subscriber.Subscribe, options...)
	if subscriber.Subscribe.BufferTime > 0 {
		subscriber.Subscribe.SubMode = SUBMODE_BUFFER
	}
	_, err = p.server.Call(subscriber)
	return
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

func (p *Plugin) Push(streamPath string, url string, options ...any) (pusher *Pusher, err error) {
	pusher = &Pusher{Push: p.config.Push}
	pusher.Client.PubSubBase = &pusher.PubSubBase
	pusher.Client.Proxy = p.config.Push.Proxy
	pusher.Client.RemoteURL = url
	pusher.Subscribe = p.config.Subscribe
	pusher.StreamPath = streamPath
	for _, option := range options {
		switch v := option.(type) {
		case PushHandler:
			defer func() {
				if err == nil {
					pusher.Start(v)
				}
			}()
		}
	}
	pusher.Init(p, streamPath, &pusher.Subscribe, options...)
	_, err = p.server.Call(pusher)
	return
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
	if p.server != p.handler {
		pattern = "/" + strings.ToLower(p.Meta.Name) + pattern
		p.Debug("http handle added to server", "pattern", pattern)
		p.server.config.HTTP.Handle(pattern, handler, last)
	}
	p.server.apiList = append(p.server.apiList, pattern)
}

func (p *Plugin) PostToServer(event any) {
	p.server.PostMessage(event)
}

func (p *Plugin) SaveConfig() (err error) {
	_, err = p.server.Call(func() error {
		if p.Modify == nil {
			os.Remove(p.settingPath())
			return nil
		}
		file, err := os.OpenFile(p.settingPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err == nil {
			defer file.Close()
			err = yaml.NewEncoder(file).Encode(p.Modify)
		}
		if err == nil {
			p.Info("config saved")
		}
		return err
	})
	return
}
