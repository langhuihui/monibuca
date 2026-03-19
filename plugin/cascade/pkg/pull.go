package cascade

import (
	"fmt"

	"github.com/quic-go/quic-go"
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	flv "m7s.live/v5/plugin/flv/pkg"
)

type Puller struct {
	flv.Puller
	*quic.Conn
}

func (p *Puller) GetPullJob() *m7s.PullJob {
	return &p.PullJob
}

func NewCascadePuller(config.Pull) m7s.IPuller {
	return &Puller{}
}

func (p *Puller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}
	var stream *quic.Stream
	stream, err = p.Conn.OpenStream()
	if err != nil {
		return
	}
	p.ReadCloser = stream
	_, err = fmt.Fprintf(stream, "%s %s\r\n", "PULLFLV", p.PullJob.Publisher.StreamPath)
	return
}

// Dispose 发送 STOPFLV 命令，然后关闭 Stream
func (p *Puller) Dispose() {
	if p.ReadCloser != nil {
		// 先发送停止命令
		if w, ok := p.ReadCloser.(interface{ Write([]byte) (int, error) }); ok {
			fmt.Fprintf(w, "STOPFLV %s\r\n", p.PullJob.Publisher.StreamPath)
			fmt.Printf("[cascade] 发送 STOPFLV: %s\n", p.PullJob.Publisher.StreamPath)
		}
		// 关闭 Stream
		p.ReadCloser.Close()
		p.ReadCloser = nil
	}
}

// Dispose 发送 STOPFLV 命令，然后关闭 Stream
func (p *Puller) Dispose() {
	if p.ReadCloser != nil {
		// 先发送停止命令
		if w, ok := p.ReadCloser.(interface{ Write([]byte) (int, error) }); ok {
			fmt.Fprintf(w, "STOPFLV %s\r\n", p.PullJob.Publisher.StreamPath)
			fmt.Printf("[cascade] 发送 STOPFLV: %s\n", p.PullJob.Publisher.StreamPath)
		}
		// 关闭 Stream
		p.ReadCloser.Close()
		p.ReadCloser = nil
	}
}
