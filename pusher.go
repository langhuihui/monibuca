package m7s

import (
	"time"

	"m7s.live/m7s/v5/pkg"

	"m7s.live/m7s/v5/pkg/config"
)

type Pusher = func(*PushContext) error

func createPushContext(p *Plugin, streamPath string, url string) (pushCtx *PushContext) {
	pushCtx = &PushContext{Push: p.config.Push}
	pushCtx.Plugin = p
	pushCtx.RemoteURL = url
	pushCtx.StreamPath = streamPath
	pushCtx.ConnectProxy = p.config.Push.Proxy
	pushCtx.Logger = p.Logger.With("pushURL", url, "streamPath", streamPath)
	return
}

type PushContext struct {
	Connection
	Subscriber *Subscriber
	config.Push
}

func (p *PushContext) GetKey() string {
	return p.RemoteURL
}

func (p *PushContext) Do(pusher Pusher) {
	p.AddCall(func(tmpTask *pkg.Task) (err error) {
		if p.Subscriber != nil && time.Since(p.Subscriber.StartTime) < 5*time.Second {
			time.Sleep(5 * time.Second)
		}
		if p.Subscriber, err = p.Plugin.Subscribe(tmpTask.Context, p.StreamPath); err != nil {
			p.Error("push subscribe failed", "error", err)
			return
		}
		err = pusher(p)
		if p.Connection.reconnect(p.RePush) {
			if time.Since(tmpTask.StartTime) < 5*time.Second {
				time.Sleep(5 * time.Second)
			}
			p.Warn("retry", "count", p.ReConnectCount, "total", p.RePush)
			p.Do(pusher)
		} else {
			p.Stop(pkg.ErrRetryRunOut)
		}
		return
	}, nil)
}

func (p *PushContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pushs.Get(p.GetKey()); ok {
		return pkg.ErrPushRemoteURLExist
	}
	s.Pushs.Add(p)
	if p.Plugin.Meta.Pusher != nil {
		p.Do(p.Plugin.Meta.Pusher)
	}
	return
}

func (p *PushContext) Dispose() {
	p.Plugin.Server.Pushs.Remove(p)
}
