package rtmp

import "m7s.live/m7s/v5"

type RTMPPlugin struct {
	m7s.Plugin
}

func (p *RTMPPlugin) OnInit() {

}

func (p *RTMPPlugin) OnStopPublish(puber *m7s.Publisher, err error) {

}

func (p *RTMPPlugin) OnEvent(event any) {
	// ...
}

var _ = m7s.InstallPlugin[*RTMPPlugin]()
