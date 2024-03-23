package m7s

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/logrusorgru/aurora/v4"
	"gopkg.in/yaml.v3"
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
	p := reflect.ValueOf(instance).Elem().FieldByName("Plugin").Addr().Interface().(*Plugin)
	p.handler = instance
	p.Meta = plugin
	p.server = s
	p.eventChan = make(chan any, 10)
	p.Logger = s.Logger.With("plugin", plugin.Name)
	p.Context, p.CancelCauseFunc = context.WithCancelCause(s.Context)
	s.Plugins = append(s.Plugins, p)
	if os.Getenv(strings.ToUpper(plugin.Name)+"_ENABLE") == "false" {
		p.Disabled = true
		p.Warn("disabled by env")
		return
	}

	p.Config.Parse(instance, strings.ToUpper(plugin.Name))
	for _, fname := range MergeConfigs {
		if name := strings.ToLower(fname); p.Config.Has(name) {
			p.Config.Get(name).ParseGlobal(s.Config.Get(name))
		}
	}
	if plugin.defaultYaml != "" {
		if err := yaml.Unmarshal([]byte(plugin.defaultYaml), &userConfig); err != nil {
			p.Error("parsing default config error:", err)
		} else {
			p.Config.ParseDefaultYaml(userConfig)
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
	instance.OnInit()
	go p.Start()
}

type IPlugin interface {
	OnInit()
	OnEvent(any)
}

type IPublishPlugin interface {
	OnStopPublish(*Publisher, error)
}

type ITCPPlugin interface {
	OnTCPConnect(*net.TCPConn)
}

var plugins []PluginMeta

func InstallPlugin[C IPlugin](options ...any) error {
	var c C
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

func sendPromiseToServer[T any](server *Server, value T) error {
	promise := util.NewPromise(value)
	server.eventChan <- promise
	<-promise.Done()
	return context.Cause(promise.Context)
}

type Plugin struct {
	Disabled                bool
	Meta                    *PluginMeta
	context.Context         `json:"-" yaml:"-"`
	context.CancelCauseFunc `json:"-" yaml:"-"`
	eventChan               chan any
	config                  config.Common
	config.Config
	Publishers []*Publisher
	*slog.Logger
	handler IPlugin
	server  *Server
	sync.RWMutex
}

func (p *Plugin) GetCommonConf() *config.Common {
	return &p.config
}

func (opt *Plugin) settingPath() string {
	return filepath.Join(opt.server.SettingDir, strings.ToLower(opt.Meta.Name)+".yaml")
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
	// p.registerHandler()
}

func (p *Plugin) Start() {
	var err error
	httpConf := p.config.HTTP
	defer httpConf.StopListen()
	if httpConf.ListenAddrTLS != "" && (httpConf.ListenAddrTLS != p.server.config.HTTP.ListenAddrTLS) {
		go func() {
			p.Info("https listen at ", "addr", aurora.Blink(httpConf.ListenAddrTLS))
			p.CancelCauseFunc(httpConf.ListenTLS())
		}()
	}
	if httpConf.ListenAddr != "" && (httpConf.ListenAddr != p.server.config.HTTP.ListenAddr) {
		go func() {
			p.Info("http listen at ", "addr", aurora.Blink(httpConf.ListenAddr))
			p.CancelCauseFunc(httpConf.Listen())
		}()
	}
	tcpConf := p.config.TCP
	tcphandler, ok := p.handler.(ITCPPlugin)
	if !ok {
		tcphandler = p
	}
	count := p.config.TCP.ListenNum
	if count == 0 {
		count = runtime.NumCPU()
	}
	if p.config.TCP.ListenAddr != "" {
		l, err := net.Listen("tcp", tcpConf.ListenAddr)
		if err != nil {
			p.Error("tcp listen error", "addr", tcpConf.ListenAddr, "error", err)
			p.CancelCauseFunc(err)
			return
		}
		defer l.Close()
		p.Info("tcp listen at ", "addr", aurora.Blink(tcpConf.ListenAddr))
		for i := 0; i < count; i++ {
			go tcpConf.Listen(l, tcphandler.OnTCPConnect)
		}
	}
	if tcpConf.ListenAddrTLS != "" {
		keyPair, _ := tls.X509KeyPair(config.LocalCert, config.LocalKey)
		if tcpConf.CertFile != "" || tcpConf.KeyFile != "" {
			keyPair, err = tls.LoadX509KeyPair(tcpConf.CertFile, tcpConf.KeyFile)
		}
		if err != nil {
			p.Error("LoadX509KeyPair", "error", err)
			p.CancelCauseFunc(err)
			return
		}
		l, err := tls.Listen("tcp", tcpConf.ListenAddrTLS, &tls.Config{
			Certificates: []tls.Certificate{keyPair},
		})
		if err != nil {
			p.Error("tls tcp listen error", "addr", tcpConf.ListenAddrTLS, "error", err)
			p.CancelCauseFunc(err)
			return
		}
		defer l.Close()
		p.Info("tls tcp listen at ", "addr", aurora.Blink(tcpConf.ListenAddrTLS))
		for i := 0; i < count; i++ {
			go tcpConf.Listen(l, tcphandler.OnTCPConnect)
		}
	}
	select {
	case event := <-p.eventChan:
		p.handler.OnEvent(event)
	case <-p.Done():
		return
	}
}

func (p *Plugin) Stop(reason error) {
	p.CancelCauseFunc(reason)
}

func (p *Plugin) OnEvent(event any) {

}

func (p *Plugin) OnTCPConnect(conn *net.TCPConn) {
	p.handler.OnEvent(conn)
}

func (p *Plugin) Publish(streamPath string) (publisher *Publisher, err error) {
	publisher = &Publisher{Publish: p.config.Publish}
	publisher.Init(p, streamPath)
	publisher.Subscribers = make(map[*Subscriber]struct{})
	err = sendPromiseToServer(p.server, publisher)
	return
}

func (p *Plugin) Subscribe(streamPath string) (subscriber *Subscriber, err error) {
	subscriber = &Subscriber{Subscribe: p.config.Subscribe}
	subscriber.Init(p, streamPath)
	err = sendPromiseToServer(p.server, subscriber)
	return
}
