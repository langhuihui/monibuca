package m7s

import (
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	"time"

	"m7s.live/m7s/v5/pkg/config"
)

type IPusher interface {
	util.ITask
	GetPushContext() *PushContext
}

type Pusher = func() IPusher

type PushContext struct {
	Connection
	Subscriber *Subscriber
	config.Push
	pusher IPusher
}

func (p *PushContext) GetKey() string {
	return p.RemoteURL
}

func (p *PushContext) Init(pusher IPusher, plugin *Plugin, streamPath string, url string) *PushContext {
	p.Push = plugin.config.Push
	p.Connection.Init(plugin, streamPath, url, plugin.config.Push.Proxy)
	p.Logger = plugin.Logger.With("pushURL", url, "streamPath", streamPath)
	if pusherTask := pusher.GetTask(); pusherTask.Logger == nil {
		pusherTask.Logger = p.Logger
	}
	p.pusher = pusher
	pusher.SetRetry(plugin.config.RePush, time.Second*5)
	return p
}

func (p *PushContext) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.pusher.GetTask().Context, p.StreamPath)
	return
}

func (p *PushContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pushs.Get(p.GetKey()); ok {
		return pkg.ErrPushRemoteURLExist
	}
	s.Pushs.Add(p)
	p.AddTask(p.pusher)
	return
}

func (p *PushContext) Dispose() {
	p.Plugin.Server.Pushs.Remove(p)
}
