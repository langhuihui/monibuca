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

func createClient(p *m7s.Connection) (s *Stream, err error) {
	addr := p.RemoteURL
	var rtspURL *url.URL
	rtspURL, err = url.Parse(addr)
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
	s = &Stream{NetConnection: NewNetConnection(conn)}
	s.Logger = p.Logger.With("local", conn.LocalAddr().String())
	s.URL = rtspURL
	s.auth = util.NewAuth(s.URL.User)
	s.Backchannel = true
	err = s.Options()
	if err != nil {
		s.Dispose()
		return
	}
	return
}

type Client struct {
	*Stream
	pullCtx   m7s.PullJob
	pushCtx   m7s.PushJob
	direction string
}

func (c *Client) Start() (err error) {
	c.Stream, err = createClient(&c.pullCtx.Connection)
	if err == nil {
		if c.direction == DIRECTION_PULL {
			err = c.pullCtx.Publish()
		}
	}
	return
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

func (c *Client) Run() (err error) {
	if c.direction == DIRECTION_PULL {
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
