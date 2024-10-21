package rtsp

import (
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"

	"m7s.live/v5"
)

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
	if c.direction == DIRECTION_PULL {
		err = c.NetConnection.Connect(c.pullCtx.RemoteURL)
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
		err = c.pullCtx.Publish()
		if err != nil {
			return
		}
		var media []*Media
		if media, err = c.Describe(); err != nil {
			return
		}
		receiver := &Receiver{Publisher: c.pullCtx.Publisher, Stream: c.Stream}
		if err = receiver.SetMedia(media); err != nil {
			return
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
