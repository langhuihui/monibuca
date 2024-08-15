package m7s

import (
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	"time"

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

type PushSubTask struct {
	util.Task
	ctx *PushContext
	Pusher
}

func (p *PushSubTask) Start() (err error) {
	if p.ctx.Subscriber, err = p.ctx.Plugin.Subscribe(p.Context, p.ctx.StreamPath); err != nil {
		p.Error("push subscribe failed", "error", err)
		return
	}
	return p.Pusher(p.ctx)
}

func (p *PushContext) Do(pusher Pusher) {
	task := &PushSubTask{ctx: p, Pusher: pusher}
	task.SetRetry(p.RePush, time.Second*5)
	p.AddTask(task)
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
