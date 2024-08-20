package cascade

import (
	"fmt"
	"github.com/quic-go/quic-go"
	"m7s.live/m7s/v5"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
)

type Puller struct {
	flv.Puller
	quic.Connection
}

func (p *Puller) GetPullContext() *m7s.PullContext {
	return &p.Ctx
}

func NewCascadePuller() m7s.IPuller {
	return &Puller{}
}

func (p *Puller) Start() (err error) {
	if err = p.Ctx.Publish(); err != nil {
		return
	}
	var stream quic.Stream
	stream, err = p.Connection.OpenStream()
	if err != nil {
		return
	}
	p.ReadCloser = stream
	_, err = fmt.Fprintf(stream, "%s %s\r\n", "PULLFLV", p.Ctx.Publisher.StreamPath)
	return
}
