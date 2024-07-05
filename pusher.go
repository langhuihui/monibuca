package m7s

import (
	"io"
	"time"

	"m7s.live/m7s/v5/pkg/config"
)

type PushHandler interface {
	Connect(*Client) error
	// Disconnect()
	Push(*Pusher) error
}

type Pusher struct {
	Client Client
	Subscriber
	config.Push
}

func (p *Pusher) GetKey() string {
	return p.Client.RemoteURL
}

func (p *Pusher) Start(handler PushHandler) (err error) {
	badPuller := true
	var startTime time.Time
	for p.Info("start push", "url", p.Client.RemoteURL); p.Client.reconnect(p.RePush); p.Warn("restart push") {
		if time.Since(startTime) < 5*time.Second {
			time.Sleep(5 * time.Second)
		}
		startTime = time.Now()
		if err = handler.Connect(&p.Client); err != nil {
			if err == io.EOF {
				p.Info("push complete")
				return
			}
			p.Error("push connect", "error", err)
			if badPuller {
				return
			}
		} else {
			badPuller = false
			p.Client.ReConnectCount = 0
			if err = handler.Push(p); err != nil && !p.IsStopped() {
				p.Error("push interrupt", "error", err)
			}
		}
		if p.IsStopped() {
			p.Info("stop push")
			return
		}
		// handler.Disconnect()
	}
	return nil
}
