package rtsp

import (
	"crypto/tls"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"net"
	"net/url"
	"strings"
)

type Client struct{}

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
	s = &Stream{NetConnection: NewNetConnection(conn, p.Logger)}
	s.URL = rtspURL
	s.auth = util.NewAuth(s.URL.User)
	s.Backchannel = true
	err = s.Options()
	if err != nil {
		s.disconnect()
		return
	}
	return
}

func (Client) DoPull(p *m7s.PullContext) (err error) {
	var s *Stream
	if s, err = createClient(&p.Connection); err != nil {
		return
	}
	defer func() {
		s.disconnect()
		if p := recover(); p != nil {
			err = p.(error)
		}
	}()
	var media []*Media
	if media, err = s.Describe(); err != nil {
		return
	}
	receiver := &Receiver{Publisher: p.Publisher, Stream: s}
	if err = receiver.SetMedia(media); err != nil {
		return
	}
	if err = s.Play(); err != nil {
		return
	}
	p.Connection.ReConnectCount = 0
	return receiver.Receive()
}

func (Client) DoPush(ctx *m7s.PushContext) (err error) {
	var s *Stream
	if s, err = createClient(&ctx.Connection); err != nil {
		return
	}
	defer s.disconnect()
	sender := &Sender{Subscriber: ctx.Subscriber, Stream: s}
	var medias []*Media
	medias, err = sender.GetMedia()
	err = s.Announce(medias)
	if err != nil {
		return
	}
	for i, media := range medias {
		switch media.Kind {
		case "audio", "video":
			_, err = s.SetupMedia(media, i)
			if err != nil {
				return
			}
		default:
			ctx.Warn("media kind not support", "kind", media.Kind)
		}
	}
	if err = s.Record(); err != nil {
		return
	}
	ctx.Connection.ReConnectCount = 0
	return sender.Send()
}
