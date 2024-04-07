package m7s

import (
	"io"
	"time"

	"m7s.live/m7s/v5/pkg/config"
)

type PullHandler interface {
	Connect() error
	Disconnect()
	Pull() error
}

type Puller struct {
	Publisher
	PullHandler
	config.Pull
	RemoteURL      string // 远程服务器地址（用于推拉）
	ReConnectCount int    //重连次数
}

// 是否需要重连
func (p *Puller) reconnect() (ok bool) {
	ok = p.RePull == -1 || p.ReConnectCount <= p.RePull
	p.ReConnectCount++
	return
}

func (p *Puller) Start(handler PullHandler) (err error) {
	p.PullHandler = handler
	badPuller := true
	var startTime time.Time
	for p.Info("start pull"); p.reconnect(); p.Warn("restart pull") {
		if time.Since(startTime) < 5*time.Second {
			time.Sleep(5 * time.Second)
		}
		startTime = time.Now()
		if err = p.Connect(); err != nil {
			if err == io.EOF {
				p.Info("pull complete")
				return
			}
			p.Error("pull connect", "error", err)
			if badPuller {
				return
			}
		} else {
			badPuller = false
			p.ReConnectCount = 0
			if err = handler.Pull(); err != nil && !p.IsStopped() {
				p.Error("pull interrupt", "error", err)
			}
		}
		if p.IsStopped() {
			p.Info("stop pull")
			return
		}
		handler.Disconnect()
	}
	return nil
}

func (p *Puller) Stop(err error) {
	p.Disconnect()
	p.Publisher.Stop(err)
}
