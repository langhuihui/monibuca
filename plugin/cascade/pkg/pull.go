package cascade

import (
	"fmt"
	"github.com/quic-go/quic-go"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
)

type Puller struct {
	flv.Puller
	quic.Connection
	quic.Stream
}

func (p *Puller) GetPullContext() *m7s.PullContext {
	return &p.Ctx
}

func NewCascadePuller() m7s.IPuller {
	return &Puller{}
}

func (p *Puller) Dispose() {
	p.Stream.Close()
}

func (p *Puller) Start() (err error) {
	if err = p.Ctx.Publish(); err != nil {
		return
	}
	p.Stream, err = p.Connection.OpenStream()
	if err != nil {
		return
	}
	p.BufReader = util.NewBufReader(p.Stream)
	fmt.Fprintf(p, "%s %s\r\n", "PULLFLV", p.Ctx.Publisher.StreamPath)
	return
}
