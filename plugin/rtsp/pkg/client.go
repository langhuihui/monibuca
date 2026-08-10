package rtsp

import (
	"errors"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5/pkg/config"

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
		if err = c.NetConnection.Connect(c.pullCtx.Context, c.pullCtx.RemoteURL); err != nil {
			c.pullCtx.Fail(err.Error())
			return
		}
	} else {
		err = c.NetConnection.Connect(c.pushCtx.Context, c.pushCtx.RemoteURL)
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

// maybeStopRetryOnAuthFail 在开启 stopRetryOnAuthFail 且错误为鉴权失败时，将 MaxRetry 置 0 以立即停重试。
// Confirmed via 寸止(REQ-RTSP-001)：不依赖厂商文案，仅识别 ErrInvalidCredentials。
func (c *Client) maybeStopRetryOnAuthFail(err error) error {
	if err == nil || c.direction != DIRECTION_PULL {
		return err
	}
	if c.pullCtx.Pull == nil || !c.pullCtx.StopRetryOnAuthFail {
		return err
	}
	if !errors.Is(err, pkg.ErrInvalidCredentials) {
		return err
	}
	c.SetRetry(0, 0)
	c.Warn("auth failed, stop retry", "streamPath", c.pullCtx.StreamPath, "url", c.pullCtx.RemoteURL, "error", err)
	return err
}

func (c *Client) Run() (err error) {
	if err = c.maybeStopRetryOnAuthFail(c.Options()); err != nil {
		return
	}
	if c.direction == DIRECTION_PULL {
		c.pullCtx.GoToStepConst(StepDescribe)

		var medias []*Media
		if medias, err = c.Describe(); err != nil {
			return c.maybeStopRetryOnAuthFail(err)
		}
		receiver := &Receiver{Publisher: c.pullCtx.Publisher, Stream: c.Stream}
		if err = receiver.SetMedia(medias); err != nil {
			return
		}

		c.pullCtx.GoToStepConst(StepSetup)

		for i, media := range medias {
			switch media.Kind {
			case "audio", "video":
				_, err = c.SetupMedia(media, "play", i)
				if err != nil {
					return
				}
			default:
				c.Warn("media kind not support", "kind", media.Kind)
			}
		}

		c.pullCtx.GoToStepConst(StepPlay)

		if err = c.Play(); err != nil {
			return
		}

		c.pullCtx.GoToStepConst(pkg.StepStreaming)

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
				_, err = c.SetupMedia(media, "record", i)
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
