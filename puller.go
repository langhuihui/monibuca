package m7s

import (
	"io"
	"time"

	"m7s.live/m7s/v5/pkg/config"
)

type Client struct {
	*PubSubBase
	RemoteURL      string // 远程服务器地址（用于推拉）
	ReConnectCount int    //重连次数
	Proxy          string // 代理地址
}

func (client *Client) reconnect(count int) (ok bool) {
	ok = count == -1 || client.ReConnectCount <= count
	client.ReConnectCount++
	return
}

type PullHandler interface {
	Connect(*Client) error
	// Disconnect()
	Pull(*Puller) error
}

type Puller struct {
	Client Client
	Publisher
	config.Pull
}

func (p *Puller) Start(handler PullHandler) (err error) {
	badPuller := true
	var startTime time.Time
	for p.Info("start pull", "url", p.Client.RemoteURL); p.Client.reconnect(p.RePull); p.Warn("restart pull") {
		if time.Since(startTime) < 5*time.Second {
			time.Sleep(5 * time.Second)
		}
		startTime = time.Now()
		if err = handler.Connect(&p.Client); err != nil {
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
			p.Client.ReConnectCount = 0
			if err = handler.Pull(p); err != nil && !p.IsStopped() {
				p.Error("pull interrupt", "error", err)
			}
		}
		if p.IsStopped() {
			p.Info("stop pull")
			return
		}
		// handler.Disconnect()
	}
	return nil
}
