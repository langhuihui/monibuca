package m7s

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/logrusorgru/aurora/v4"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type PluginMeta struct {
	Name    string
	Version string //插件版本
	Type    reflect.Type
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
		Name: t.Name(),
		Type: t,
	}

	_, pluginFilePath, _, _ := runtime.Caller(1)
	configDir := filepath.Dir(pluginFilePath)

	if _, after, found := strings.Cut(configDir, "@"); found {
		meta.Version = after
	} else {
		meta.Version = pluginFilePath
	}
	plugins = append(plugins, meta)
	return nil
}

func sendPromiseToServer[T any](server *Server, value T) error {
	promise := util.NewPromise(value)
	server.EventBus <- promise
	<-promise.Done()
	return context.Cause(promise.Context)
}

type Plugin struct {
	Disabled                bool
	Meta                    *PluginMeta
	context.Context         `json:"-" yaml:"-"`
	context.CancelCauseFunc `json:"-" yaml:"-"`
	Config                  struct {
		config.Publish
		config.Subscribe
		config.HTTP
		config.Quic
		config.TCP
		config.Pull
		config.Push
	}
	Publishers []*Publisher
	*slog.Logger
	handler IPlugin
	server  *Server
	sync.RWMutex
}

func (p *Plugin) OnInit() {
	var err error
	httpConf := p.Config.HTTP
	defer httpConf.StopListen()
	if httpConf.ListenAddrTLS != "" && (httpConf.ListenAddrTLS != p.server.Config.ListenAddrTLS) {
		go func() {
			p.Info("https listen at ", "addr", aurora.Blink(httpConf.ListenAddrTLS))
			p.CancelCauseFunc(httpConf.ListenTLS())
		}()
	}
	if httpConf.ListenAddr != "" && (httpConf.ListenAddr != p.server.Config.ListenAddr) {
		go func() {
			p.Info("http listen at ", "addr", aurora.Blink(httpConf.ListenAddr))
			p.CancelCauseFunc(httpConf.Listen())
		}()
	}
	tcpConf := p.Config.TCP
	tcphandler, ok := p.handler.(ITCPPlugin)
	if !ok {
		tcphandler = p
	}
	count := p.Config.TCP.ListenNum
	if count == 0 {
		count = runtime.NumCPU()
	}
	if p.Config.TCP.ListenAddr != "" {
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
	case <-p.Done():
		return
	}
}

func (p *Plugin) OnEvent(event any) {

}

func (p *Plugin) OnTCPConnect(conn *net.TCPConn) {
	p.handler.OnEvent(conn)
}

func (p *Plugin) Publish(streamPath string) (publisher *Publisher, err error) {
	publisher = &Publisher{Publish: p.Config.Publish}
	publisher.Init(p, streamPath)
	publisher.Subscribers = make(map[*Subscriber]struct{})
	err = sendPromiseToServer(p.server, publisher)
	return
}

func (p *Plugin) Subscribe(streamPath string) (subscriber *Subscriber, err error) {
	subscriber = &Subscriber{Subscribe: p.Config.Subscribe}
	subscriber.Init(p, streamPath)
	err = sendPromiseToServer(p.server, subscriber)
	return
}
