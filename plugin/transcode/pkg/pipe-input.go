package transcode

import (
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/pkg/util"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
	"net"
)

type PipeInput struct {
	task.Task
	rBuf chan net.Buffers
	*util.BufReader
	flv.Live
}

func (p *PipeInput) Start() (err error) {
	p.rBuf = make(chan net.Buffers, 100)
	p.BufReader = util.NewBufReaderBuffersChan(p.rBuf)
	p.rBuf <- net.Buffers{flv.FLVHead}
	p.WriteFlvTag = func(flv net.Buffers) (err error) {
		select {
		case p.rBuf <- flv:
		default:
			p.Warn("pipe input buffer full")
		}
		return
	}
	return
}

func (p *PipeInput) Dispose() {
	close(p.rBuf)
	p.BufReader.Recycle()
}
