package m7s

import (
	"m7s.live/v5/pkg/task"

	"m7s.live/v5/pkg/config"
)

type IPusher interface {
	task.ITask
	GetPushJob() *PushJob
}

type PusherFactory = func() IPusher

type PushJob struct {
	Connection
	Subscriber *Subscriber
	SubConf    config.Subscribe
	pusher     IPusher
}

func (p *PushJob) GetKey() string {
	return p.Connection.RemoteURL
}

func (p *PushJob) Init(pusher IPusher, plugin *Plugin, streamPath string, conf config.Push, subConf *config.Subscribe) *PushJob {
	p.Connection.Init(plugin, streamPath, conf.URL, conf.Proxy)
	p.pusher = pusher
	if subConf == nil {
		p.SubConf = plugin.config.Subscribe
	} else {
		p.SubConf = *subConf
	}
	p.SubConf.SubType = SubscribeTypePush
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"url":        conf.URL,
		"maxRetry":   conf.MaxRetry,
	})
	pusher.SetRetry(conf.MaxRetry, conf.RetryInterval)
	if sender, webhook := plugin.getHookSender(config.HookOnPushStart); sender != nil {
		pusher.OnStart(func() {
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnPushStart),
				AlarmDesc:  "start push",
				AlarmType:  config.AlarmPushRecover,
				StreamPath: streamPath,
			}
			sender(webhook, alarmInfo)
		})
	}

	if sender, webhook := plugin.getHookSender(config.HookOnPushEnd); sender != nil {
		pusher.OnDispose(func() {
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnPushEnd),
				AlarmDesc:  pusher.StopReason().Error(),
				AlarmType:  config.AlarmPushOffline,
				StreamPath: streamPath,
			}
			sender(webhook, alarmInfo)
		})
	}
	plugin.Server.Pushs.AddTask(p, plugin.Logger.With("pushURL", conf.URL, "streamPath", streamPath))
	return p
}

func (p *PushJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.SubscribeWithConfig(p.pusher.GetTask().Context, p.StreamPath, p.SubConf)
	return
}

func (p *PushJob) Start() (err error) {
	p.AddTask(p.pusher, p.Logger)
	return
}
