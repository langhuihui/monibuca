package m7s

import (
	"context"
	"time"

	"m7s.live/m7s/v5/pkg"

	"m7s.live/m7s/v5/pkg/config"
)

type Pusher = func(*PushContext) error

func createPushContext(p *Plugin, streamPath string, url string, options ...any) (pushCtx *PushContext) {
	pushCtx = &PushContext{Push: p.config.Push}
	pushCtx.ID = p.Server.pushTask.GetID()
	pushCtx.Plugin = p
	pushCtx.RemoteURL = url
	pushCtx.StreamPath = streamPath
	pushCtx.ConnectProxy = p.config.Push.Proxy
	pushCtx.SubscribeOptions = []any{p.config.Subscribe}
	var ctx = p.Context
	for _, option := range options {
		switch v := option.(type) {
		case context.Context:
			ctx = v
		default:
			pushCtx.SubscribeOptions = append(pushCtx.SubscribeOptions, option)
		}
	}
	pushCtx.Init(ctx, p.Logger.With("pushURL", url, "streamPath", streamPath), pushCtx)
	pushCtx.SubscribeOptions = append(pushCtx.SubscribeOptions, pushCtx.Context)
	return
}

type PushContext struct {
	Connection
	Subscriber       *Subscriber
	SubscribeOptions []any
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
		if p.Subscriber, err = p.Plugin.Subscribe(p.StreamPath, p.SubscribeOptions...); err != nil {
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
