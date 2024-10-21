package webrtc

import (
	"m7s.live/v5"
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
	pullCtx   m7s.PullJob
	pushCtx   m7s.PushJob
	direction string
	appId     string
	token     string
	apiBase   string
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *Client) GetPushJob() *m7s.PushJob {
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
