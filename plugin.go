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

	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v3"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type DefaultYaml string

type PluginMeta struct {
	Name        string
	Version     string //插件版本
	Type        reflect.Type
	defaultYaml DefaultYaml //默认配置
}

func (plugin *PluginMeta) Init(s *Server, userConfig map[string]any) {
	instance := reflect.New(plugin.Type).Interface().(IPlugin)
	defaults.SetDefaults(instance)
	p := reflect.ValueOf(instance).Elem().FieldByName("Plugin").Addr().Interface().(*Plugin)
	p.handler = instance
	p.Meta = plugin
	p.server = s
	p.Logger = s.Logger.With("plugin", plugin.Name)
	p.Context, p.CancelCauseFunc = context.WithCancelCause(s.Context)
	s.Plugins = append(s.Plugins, p)
	if os.Getenv(strings.ToUpper(plugin.Name)+"_ENABLE") == "false" {
		p.Disabled = true
		p.Warn("disabled by env")
		return
	}
	p.Config.Parse(p.GetCommonConf())
	p.Config.Parse(instance, strings.ToUpper(plugin.Name))
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
	} else {
		p.assign()
	}
	p.Info("init", "version", plugin.Version)
	instance.OnInit()
	p.Start()
}

type iPlugin interface {
	nothing()
}

type IPlugin interface {
	OnInit()
	OnEvent(any)
}

type ITCPPlugin interface {
	OnTCPConnect(*net.TCPConn)
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
		}
	}
	plugins = append(plugins, meta)
	return nil
}

func sendPromiseToServer[T any](server *Server, value T) (err error) {
	promise := util.NewPromise(value)
	server.eventChan <- promise
	<-promise.Done()
	if err = context.Cause(promise.Context); err == util.ErrResolve {
		err = nil
	}
	return
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
	p.registerHandler()
}

func (p *Plugin) Stop(err error) {
	p.Unit.Stop(err)
	p.config.HTTP.StopListen()
	p.config.TCP.StopListen()
}

func (p *Plugin) Start() {
	httpConf := p.config.HTTP
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
	tcpConf := p.config.TCP
	tcphandler, ok := p.handler.(ITCPPlugin)
	if !ok {
		tcphandler = p
	}

	if p.config.TCP.ListenAddr != "" {
		p.Info("listen tcp", "addr", tcpConf.ListenAddr)
		go func() {
			err := tcpConf.Listen(tcphandler.OnTCPConnect)
			if err != nil {
				p.Error("listen tcp", "addr", tcpConf.ListenAddr, "error", err)
				p.Stop(err)
			}
		}()
	}
	if tcpConf.ListenAddrTLS != "" {
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

func (p *Plugin) OnInit() {

}

func (p *Plugin) onEvent(event any) {
	switch v := event.(type) {
	case *Publisher:
		if h, ok := p.handler.(interface{ OnPublish(*Publisher) }); ok {
			h.OnPublish(v)
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
	publisher.Init(p, streamPath, options...)
	return publisher, sendPromiseToServer(p.server, publisher)
}

func (p *Plugin) Pull(streamPath string, url string, options ...any) (puller *Puller, err error) {
	puller = &Puller{Pull: p.config.Pull}
	puller.Publish = p.config.Publish
	puller.PublishTimeout = 0
	puller.RemoteURL = url
	puller.StreamPath = streamPath
	puller.Init(p, streamPath, options...)
	return puller, sendPromiseToServer(p.server, puller)
}

func (p *Plugin) Subscribe(streamPath string, options ...any) (subscriber *Subscriber, err error) {
	subscriber = &Subscriber{Subscribe: p.config.Subscribe}
	subscriber.Init(p, streamPath, options...)
	return subscriber, sendPromiseToServer(p.server, subscriber)
}

func (p *Plugin) registerHandler() {
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
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	handler = p.logHandler(handler)
	p.GetCommonConf().Handle(pattern, handler)
	if p.server != p.handler {
		pattern = "/" + strings.ToLower(p.Meta.Name) + pattern
		p.Debug("http handle added to server", "pattern", pattern)
		p.server.GetCommonConf().Handle(pattern, handler)
	}
	p.server.apiList = append(p.server.apiList, pattern)
}
