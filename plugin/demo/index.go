package demo

import (
	"m7s.live/m7s/v5"
)

type DemoPlugin struct {
	m7s.Plugin
}

func (p *DemoPlugin) OnInit() {
	// puber, err := p.Publish("live/demo")
	// if err != nil {
	// 	return
	// }
	// puber.WriteVideo(&rtmp.RTMPVideo{
	// 	Timestamp: 0,
	// 	Buffers:   net.Buffers{[]byte{0x17, 0x00, 0x67, 0x42, 0x00, 0x0a, 0x8f, 0x14, 0x01, 0x00, 0x00, 0x03, 0x00, 0x80, 0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x3c, 0x80}},
	// })
}

func (p *DemoPlugin) OnStopPublish(puber *m7s.Publisher, err error) {

}

var _ = m7s.InstallPlugin[*DemoPlugin]()
