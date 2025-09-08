package plugin_hiksdk

import (
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
)

const (
	DIRECTION_PULL = "pull"
	DIRECTION_PUSH = "push"
)

type ClientPlugin struct {
	task.Job
	conf      	*HikPlugin

	pullCtx   m7s.PullJob
	pushCtx   m7s.PushJob
	direction string
}

func (c *ClientPlugin) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *ClientPlugin) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller(_ config.Pull) m7s.IPuller {
	client := &ClientPlugin{
		direction: DIRECTION_PULL,
	}
	client.SetDescription(task.OwnerTypeKey, "HikPuller")
	return client
}

func NewPusher() m7s.IPusher {
	client := &ClientPlugin{
		direction: DIRECTION_PUSH,
	}
	client.SetDescription(task.OwnerTypeKey, "HikPusher")
	return client
}