package m7s

import (
	"context"
	"m7s.live/m7s/v5/pkg"
	"time"

	"m7s.live/m7s/v5/pkg/config"
)

type Pusher = func(*PushContext) error

func createPushContext(p *Plugin, streamPath string, url string, options ...any) (pushCtx *PushContext) {
	pushCtx = &PushContext{Push: p.config.Push}
	pushCtx.ID = p.Server.pushTM.GetID()
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

func (p *PushContext) Run(pusher Pusher) {
	p.StartTime = time.Now()
	defer p.Info("stop push")
	var err error
	for p.Info("start push", "url", p.Connection.RemoteURL); p.Connection.reconnect(p.RePush); p.Warn("restart push") {
		if p.Subscriber != nil && time.Since(p.Subscriber.StartTime) < 5*time.Second {
			time.Sleep(5 * time.Second)
		}
		if p.Subscriber, err = p.Plugin.Subscribe(p.StreamPath, p.SubscribeOptions...); err != nil {
			p.Error("push subscribe failed", "error", err)
			break
		}
		err = pusher(p)
		p.Subscriber.Stop(err)
		if p.IsStopped() {
			return
		} else {
			p.Error("push interrupt", "error", err)
		}
	}
	if err == nil {
		err = pkg.ErrRetryRunOut
	}
	p.Stop(err)
	return
}

func (p *PushContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pushs.Get(p.GetKey()); ok {
		return pkg.ErrPushRemoteURLExist
	}
	s.Pushs.Add(p)
	return
}

func (p *PushContext) Dispose() {
	p.Plugin.Server.Pushs.Remove(p)
}
