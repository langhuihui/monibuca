package cascade

import (
	"fmt"

	"github.com/quic-go/quic-go"
	"m7s.live/v5"
	flv "m7s.live/v5/plugin/flv/pkg"
)

type Puller struct {
	flv.Puller
	quic.Connection
}

func (p *Puller) GetPullJob() *m7s.PullJob {
	return &p.PullJob
}

func NewCascadePuller() m7s.IPuller {
	return &Puller{}
}

func (p *Puller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}
	var stream quic.Stream
	stream, err = p.Connection.OpenStream()
	if err != nil {
		return
	}
	p.ReadCloser = stream
	_, err = fmt.Fprintf(stream, "%s %s\r\n", "PULLFLV", p.PullJob.Publisher.StreamPath)
	return
}
