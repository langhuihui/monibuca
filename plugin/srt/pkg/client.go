package srt

import (
	"net/url"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
)

type Client struct {
	task.Task
	srt.Conn
	srt.ConnType
	pullCtx m7s.PullJob
	pushCtx m7s.PushJob
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *Client) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller(_ config.Pull) m7s.IPuller {
	ret := &Client{
		ConnType: srt.SUBSCRIBE,
	}
	return ret
}

func NewPusher() m7s.IPusher {
	ret := &Client{
		ConnType: srt.PUBLISH,
	}
	return ret
}

func (c *Client) Start() (err error) {
	var u *url.URL
	if c.ConnType == srt.SUBSCRIBE {
		if err = c.pullCtx.Publish(); err != nil {
			return
		}
		u, err = url.Parse(c.pullCtx.RemoteURL)
	} else {
		u, err = url.Parse(c.pushCtx.RemoteURL)
	}
	if err != nil {
		return
	}
	conf := srt.DefaultConfig()
	conf.StreamId = u.Query().Get("streamid")
	conf.Passphrase = u.Query().Get("passphrase")
	c.Conn, err = srt.Dial("srt", u.Host, conf)
	return
}

func (c *Client) Run() (err error) {
	if c.ConnType == srt.SUBSCRIBE {
		var receiver Receiver
		receiver.Conn = c.Conn
		receiver.Publisher = c.pullCtx.Publisher
		c.pullCtx.AddTask(&receiver)
	} else {
		var sender Sender
		sender.Conn = c.Conn
		sender.Subscriber = c.pushCtx.Subscriber
		c.pushCtx.AddTask(&sender)
	}
	return
}
