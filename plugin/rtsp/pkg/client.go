package rtsp

import (
	"crypto/tls"
	"net"
	"net/url"
	"strings"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
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
	var rtspURL *url.URL
	if c.direction == DIRECTION_PULL {
		rtspURL, err = url.Parse(c.pullCtx.RemoteURL)
	} else {
		rtspURL, err = url.Parse(c.pushCtx.RemoteURL)
	}
	if err != nil {
		return
	}
	//ps := strings.Split(u.Path, "/")
	//if len(ps) < 3 {
	//	return errors.New("illegal rtsp url")
	//}
	istls := rtspURL.Scheme == "rtsps"
	if strings.Count(rtspURL.Host, ":") == 0 {
		if istls {
			rtspURL.Host += ":443"
		} else {
			rtspURL.Host += ":554"
		}
	}
	var conn net.Conn
	if istls {
		var tlsconn *tls.Conn
		tlsconn, err = tls.Dial("tcp", rtspURL.Host, &tls.Config{})
		conn = tlsconn
	} else {
		conn, err = net.Dial("tcp", rtspURL.Host)
	}
	if err != nil {
		return
	}
	c.conn = conn
	c.URL = rtspURL
	c.UserAgent = "monibuca" + m7s.Version
	c.auth = util.NewAuth(c.URL.User)
	c.Backchannel = true
	return
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *Client) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller() m7s.IPuller {
	client := &Client{
		direction: DIRECTION_PULL,
	}
	client.NetConnection = &NetConnection{}
	return client
}

func NewPusher() m7s.IPusher {
	client := &Client{
		direction: DIRECTION_PUSH,
	}
	client.NetConnection = &NetConnection{}
	return client
}

func (c *Client) Run() (err error) {
	c.BufReader = util.NewBufReader(c.conn)
	c.MemoryAllocator = util.NewScalableMemoryAllocator(1 << 12)
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
