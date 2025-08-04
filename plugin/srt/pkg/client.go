package srt

import (
	"net/url"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
)

// Fixed steps for SRT pull workflow
var srtPullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing stream"},
	{Name: pkg.StepConnection, Description: "Connecting to SRT server"},
	{Name: pkg.StepHandshake, Description: "Completing SRT handshake"},
	{Name: pkg.StepStreaming, Description: "Receiving SRT stream"},
}

// srt客户端
type srtClient struct {
	task.Task
	srt.Conn
}

// srt拉流客户端
type srtPuller struct {
	srtClient
	pullCtx m7s.PullJob
}

// srt推流客户端
type srtPusher struct {
	srtClient
	pushCtx m7s.PushJob
}

func (c *srtPuller) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *srtPusher) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller(_ config.Pull) m7s.IPuller {
	return &srtPuller{}
}

func NewPusher() m7s.IPusher {
	return &srtPusher{}
}

func (c *srtClient) dial(remoteURL string) (err error) {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return
	}
	conf := srt.DefaultConfig()
	conf.StreamId = u.Query().Get("streamid")
	conf.Passphrase = u.Query().Get("passphrase")
	c.Conn, err = srt.Dial("srt", u.Host, conf)
	return
}

func (c *srtPuller) Start() (err error) {
	c.pullCtx.SetProgressStepsDefs(srtPullSteps)

	if err = c.pullCtx.Publish(); err != nil {
		c.pullCtx.Fail(err.Error())
		return
	}

	c.pullCtx.GoToStepConst(pkg.StepConnection)

	err = c.dial(c.pullCtx.RemoteURL)
	if err != nil {
		c.pullCtx.Fail(err.Error())
		return
	}

	c.pullCtx.GoToStepConst(pkg.StepStreaming)

	return
}

func (c *srtPusher) Start() (err error) {
	return c.dial(c.pushCtx.RemoteURL)
}

func (c *srtPuller) Run() (err error) {
	var receiver Receiver
	receiver.Conn = c.Conn
	receiver.Publisher = c.pullCtx.Publisher
	return c.RunTask(&receiver)
}

func (c *srtPusher) Run() (err error) {
	var sender Sender
	sender.Conn = c.Conn
	sender.Subscriber = c.pushCtx.Subscriber
	return c.RunTask(&sender)
}
