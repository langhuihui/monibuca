package demo

import (
	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/pkg"
)

type DemoPlugin struct {
	m7s.Plugin
}

func (p *DemoPlugin) OnInit() {
	puber, err := p.Publish("live/demo")
	if err != nil {
		panic(err)
	}
	puber.WriteVideo(&H264Nalu{})
}

func (p *DemoPlugin) OnEvent(event any) {
	// ...
}

var _ = m7s.InstallPlugin[*DemoPlugin]()
