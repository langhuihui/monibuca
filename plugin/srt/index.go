package plugin_srt

import (
	"strings"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	pkg "m7s.live/v5/plugin/srt/pkg"
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

const defaultConfig = m7s.DefaultYaml(`listenaddr: :6000`)

var _ = m7s.InstallPlugin[SRTPlugin](defaultConfig, pkg.NewPuller, pkg.NewPusher)

func (p *SRTPlugin) OnInit() error {
	var t SRTServer
	t.server.Addr = p.ListenAddr
	t.plugin = p
	p.AddTask(&t)
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
		var receiver pkg.Receiver
		receiver.Conn = conn
		receiver.Publisher = publisher
		t.AddTask(&receiver)
	}
	t.server.HandleSubscribe = func(conn srt.Conn) {
		_, streamPath, _ := strings.Cut(conn.StreamId(), "/")
		subscriber, err := t.plugin.Subscribe(t.plugin, streamPath)
		if err != nil {
			conn.Close()
			return
		}
		var sender pkg.Sender
		sender.Conn = conn
		sender.Subscriber = subscriber
		t.AddTask(&sender)
	}
	return nil
}

func (t *SRTServer) OnStop() {
	t.server.Shutdown()
}

func (t *SRTServer) Go() error {
	return t.server.ListenAndServe()
}
