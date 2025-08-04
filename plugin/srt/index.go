package plugin_srt

import (
	"fmt"
	"strings"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	srt_pkg "m7s.live/v5/plugin/srt/pkg"
)

type SRTServer struct {
	task.Job
	server srt.Server
	plugin *SRTPlugin
}

type SRTPlugin struct {
	m7s.Plugin
	ListenAddr string
	Passphrase string
}

var _ = m7s.InstallPlugin[SRTPlugin](m7s.PluginMeta{
	DefaultYaml: `listenaddr: :6000`,
	NewPuller:   srt_pkg.NewPuller,
	NewPusher:   srt_pkg.NewPusher,
})

func (p *SRTPlugin) Start() error {
	var t SRTServer
	t.server.Addr = p.ListenAddr
	t.plugin = p
	p.AddTask(&t)
	_, port, _ := strings.Cut(p.ListenAddr, ":")
	if port == "6000" {
		p.PushAddr = append(p.PushAddr, "srt://{hostName}?streamid=publish:/{streamPath}")
		p.PlayAddr = append(p.PlayAddr, "srt://{hostName}?streamid=subscribe:/{streamPath}")
	} else if port != "" {
		p.PushAddr = append(p.PushAddr, fmt.Sprintf("srt://{hostName}:%s?streamid=publish:/{streamPath}", port))
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("srt://{hostName}:%s?streamid=subscribe:/{streamPath}", port))
	}
	return nil
}

func (t *SRTServer) Start() error {
	t.server.HandleConnect = func(conn srt.ConnRequest) srt.ConnType {
		streamid := conn.StreamId()
		conn.SetPassphrase(t.plugin.Passphrase)
		if strings.HasPrefix(streamid, "publish:") {
			return srt.PUBLISH
		}
		return srt.SUBSCRIBE
	}
	t.server.HandlePublish = func(conn srt.Conn) {
		_, streamPath, _ := strings.Cut(conn.StreamId(), "/")
		publisher, err := t.plugin.Publish(t.plugin, streamPath)
		if err != nil {
			conn.Close()
			return
		}
		var receiver srt_pkg.Receiver
		receiver.Conn = conn
		receiver.Publisher = publisher
		t.RunTask(&receiver)
	}
	t.server.HandleSubscribe = func(conn srt.Conn) {
		_, streamPath, _ := strings.Cut(conn.StreamId(), "/")
		subscriber, err := t.plugin.Subscribe(t.plugin, streamPath)
		if err != nil {
			conn.Close()
			return
		}
		var sender srt_pkg.Sender
		sender.Conn = conn
		sender.Subscriber = subscriber
		sender.Using(subscriber)
		t.RunTask(&sender)
	}
	return nil
}

func (t *SRTServer) Dispose() {
	t.server.Shutdown()
}

func (t *SRTServer) Go() error {
	return t.server.ListenAndServe()
}
