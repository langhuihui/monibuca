package demo

import (
	m7s "m7s.live/monibuca/v5"
	. "m7s.live/monibuca/v5/pkg"
)

type DemoPlugin struct {
	m7s.Plugin
}

func (p *DemoPlugin) OnInit() {
	puber := p.Publish("live/demo")
	puber.WriteVideo(&H264Nalu{})
}

func (p *DemoPlugin) OnEvent(event any) {
	// ...
}

var _ = m7s.InstallPlugin[*DemoPlugin]()
