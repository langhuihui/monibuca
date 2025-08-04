package rtsp

import (
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"

	"m7s.live/v5"
	pkg "m7s.live/v5/pkg"
)

// Plugin-specific progress step names for RTSP
const (
	StepDescribe pkg.StepName = "describe"
	StepSetup    pkg.StepName = "setup"
	StepPlay     pkg.StepName = "play"
)

// Fixed steps for RTSP pull workflow
var rtspPullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing stream"},
	{Name: pkg.StepConnection, Description: "Connecting to RTSP server"},
	{Name: StepDescribe, Description: "Sending DESCRIBE request"},
	{Name: StepSetup, Description: "Setting up media tracks"},
	{Name: StepPlay, Description: "Starting media playback"},
	{Name: pkg.StepStreaming, Description: "Receiving and processing media data"},
}

const (
	DIRECTION_PULL = "pull"
	DIRECTION_PUSH = "push"
)

type Client struct {
	Stream
	pullCtx   m7s.PullJob
	pushCtx   m7s.PushJob
	direction string
}

func (c *Client) Start() (err error) {
	if c.direction == DIRECTION_PULL { // no progress tracking
		c.pullCtx.SetProgressStepsDefs(rtspPullSteps)
		if err = c.pullCtx.Publish(); err != nil {
			c.pullCtx.Fail(err.Error())
			return
		}
		if err = c.NetConnection.Connect(c.pullCtx.RemoteURL); err != nil {
			c.pullCtx.Fail(err.Error())
			return
		}
	} else {
		err = c.NetConnection.Connect(c.pushCtx.RemoteURL)
	}
	return
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *Client) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller(_ config.Pull) m7s.IPuller {
	client := &Client{
		direction: DIRECTION_PULL,
	}
	client.NetConnection = &NetConnection{}
	client.SetDescription(task.OwnerTypeKey, "RTSPPuller")
	return client
}

func NewPusher() m7s.IPusher {
	client := &Client{
		direction: DIRECTION_PUSH,
	}
	client.NetConnection = &NetConnection{}
	client.SetDescription(task.OwnerTypeKey, "RTSPPusher")
	return client
}

func (c *Client) Run() (err error) {
	if err = c.Options(); err != nil {
		return
	}
	if c.direction == DIRECTION_PULL {
		var medias []*Media
		if medias, err = c.Describe(); err != nil {
			return
		}
		receiver := &Receiver{Publisher: c.pullCtx.Publisher, Stream: c.Stream}
		if err = receiver.SetMedia(medias); err != nil {
			return
		}
		for i, media := range medias {
			switch media.Kind {
			case "audio", "video":
				_, err = c.SetupMedia(media, i)
				if err != nil {
					return
				}
			default:
				c.Warn("media kind not support", "kind", media.Kind)
			}
		}
		if err = c.Play(); err != nil {
			return
		}
		return receiver.Receive()
	} else {
		err = c.pushCtx.Subscribe()
		if err != nil {
			return
		}
		sender := &Sender{Subscriber: c.pushCtx.Subscriber, Stream: c.Stream}
		var medias []*Media
		medias, err = sender.GetMedia()
		err = c.Announce(medias)
		if err != nil {
			return
		}
		for i, media := range medias {
			switch media.Kind {
			case "audio", "video":
				_, err = c.SetupMedia(media, i)
				if err != nil {
					return
				}
			default:
				c.Warn("media kind not support", "kind", media.Kind)
			}
		}
		if err = c.Record(); err != nil {
			return
		}
		return sender.Send()
	}
}
