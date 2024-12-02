package m7s

import (
	"net/http"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/task"

	"m7s.live/v5/pkg/config"
)

type IPusher interface {
	task.ITask
	GetPushJob() *PushJob
}

type Pusher = func() IPusher

type PushJob struct {
	Connection
	Subscriber *Subscriber
	pusher     IPusher
}

func (p *PushJob) GetKey() string {
	return p.Connection.RemoteURL
}

func (p *PushJob) Init(pusher IPusher, plugin *Plugin, streamPath string, conf config.Push) *PushJob {
	p.Connection.Init(plugin, streamPath, conf.URL, conf.Proxy, http.Header(conf.Header))
	p.pusher = pusher
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"url":        conf.URL,
		"maxRetry":   conf.MaxRetry,
	})
	pusher.SetRetry(conf.MaxRetry, conf.RetryInterval)
	plugin.Server.Pushs.Add(p, plugin.Logger.With("pushURL", conf.URL, "streamPath", streamPath))
	return p
}

func (p *PushJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.pusher.GetTask().Context, p.StreamPath)
	if p.Subscriber != nil {
		p.Subscriber.Internal = true
	}
	return
}

func (p *PushJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pushs.Get(p.GetKey()); ok {
		return pkg.ErrPushRemoteURLExist
	}
	p.AddTask(p.pusher, p.Logger)
	return
}
