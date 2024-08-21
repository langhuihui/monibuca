package webrtc

import (
	"m7s.live/m7s/v5"
)

const (
	DIRECTION_PULL = "pull"
	DIRECTION_PUSH = "push"
)

type PullRequest struct {
	Tracks []TrackInfo `json:"tracks"`
}

type Client struct {
	Connection
	pullCtx   m7s.PullContext
	pushCtx   m7s.PushContext
	direction string
	appId     string
	token     string
	apiBase   string
}

func (c *Client) GetPullContext() *m7s.PullContext {
	return &c.pullCtx
}

func (c *Client) GetPushContext() *m7s.PushContext {
	return &c.pushCtx
}

func NewPuller() m7s.IPuller {
	return &Client{
		direction: DIRECTION_PULL,
	}
}

func NewPusher() m7s.IPusher {
	return &Client{
		direction: DIRECTION_PUSH,
	}
}
