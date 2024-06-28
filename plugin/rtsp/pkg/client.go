package rtsp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	Stream
}

func (c *Client) Connect(p *m7s.Client) (err error) {
	addr := p.RemoteURL
	var rtspURL *url.URL
	rtspURL, err = url.Parse(addr)
	if err != nil {
		return err
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
		return err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	c.NetConnection = NewNetConnection(conn, p.Logger)
	c.URL = rtspURL
	c.auth = util.NewAuth(c.URL.User)
	c.Backchannel = true
	return c.Options()
}

func (c *Client) Pull(p *m7s.Puller) (err error) {
	defer func() {
		c.Close()
		if p := recover(); p != nil {
			err = p.(error)
		}
		p.Dispose(err)
	}()
	var media []*core.Media
	if media, err = c.Describe(); err != nil {
		return
	}
	receiver := &Receiver{Publisher: &p.Publisher, Stream: c.Stream}
	if err = receiver.SetMedia(media); err != nil {
		return
	}
	if err = c.Play(); err != nil {
		return
	}
	return receiver.Receive()
}

func (c *Client) Push(p *m7s.Pusher) (err error) {
	defer c.Close()
	sender := &Sender{Subscriber: &p.Subscriber, Stream: c.Stream}
	var medias []*core.Media
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

func (c *Client) Do(req *util.Request) (*util.Response, error) {
	if err := c.WriteRequest(req); err != nil {
		return nil, err
	}

	res, err := c.ReadResponse()
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized {
		switch c.auth.Method {
		case tcp.AuthNone:
			if c.auth.ReadNone(res) {
				return c.Do(req)
			}
			return nil, errors.New("user/pass not provided")
		case tcp.AuthUnknown:
			if c.auth.Read(res) {
				return c.Do(req)
			}
		default:
			return nil, errors.New("wrong user/pass")
		}
	}

	if res.StatusCode != http.StatusOK {
		return res, fmt.Errorf("wrong response on %s", req.Method)
	}

	return res, nil
}

func (c *Client) Options() error {
	req := &util.Request{Method: MethodOptions, URL: c.URL}

	res, err := c.Do(req)
	if err != nil {
		return err
	}

	if val := res.Header.Get("Content-Base"); val != "" {
		c.URL, err = urlParse(val)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Describe() (medias []*core.Media, err error) {
	// 5.3 Back channel connection
	// https://www.onvif.org/specs/stream/ONVIF-Streaming-Spec.pdf
	req := &util.Request{
		Method: MethodDescribe,
		URL:    c.URL,
		Header: map[string][]string{
			"Accept": {"application/sdp"},
		},
	}

	if c.Backchannel {
		req.Header.Set("Require", "www.onvif.org/ver20/backchannel")
	}

	if c.UserAgent != "" {
		// this camera will answer with 401 on DESCRIBE without User-Agent
		// https://github.com/AlexxIT/go2rtc/issues/235
		req.Header.Set("User-Agent", c.UserAgent)
	}
	var res *util.Response
	res, err = c.Do(req)
	if err != nil {
		return
	}

	if val := res.Header.Get("Content-Base"); val != "" {
		c.URL, err = urlParse(val)
		if err != nil {
			return
		}
	}

	c.sdp = string(res.Body) // for info

	medias, err = UnmarshalSDP(res.Body)
	if err != nil {
		return
	}
	if c.Media != "" {
		clone := make([]*core.Media, 0, len(medias))
		for _, media := range medias {
			if strings.Contains(c.Media, media.Kind) {
				clone = append(clone, media)
			}
		}
		medias = clone
	}

	return
}

func (c *Client) Announce(medias []*core.Media) (err error) {
	req := &util.Request{
		Method: MethodAnnounce,
		URL:    c.URL,
		Header: map[string][]string{
			"Content-Type": {"application/sdp"},
		},
	}

	req.Body, err = core.MarshalSDP(c.SessionName, medias)
	if err != nil {
		return err
	}

	_, err = c.Do(req)

	return
}

func (c *Client) SetupMedia(media *core.Media, index int) (byte, error) {
	var transport string
	transport = fmt.Sprintf(
		// i   - RTP (data channel)
		// i+1 - RTCP (control channel)
		"RTP/AVP/TCP;unicast;interleaved=%d-%d", index*2, index*2+1,
	)
	if transport == "" {
		return 0, fmt.Errorf("wrong media: %v", media)
	}

	rawURL := media.ID // control
	if !strings.Contains(rawURL, "://") {
		rawURL = c.URL.String()
		if !strings.HasSuffix(rawURL, "/") {
			rawURL += "/"
		}
		rawURL += media.ID
	}
	trackURL, err := urlParse(rawURL)
	if err != nil {
		return 0, err
	}

	req := &util.Request{
		Method: MethodSetup,
		URL:    trackURL,
		Header: map[string][]string{
			"Transport": {transport},
		},
	}

	res, err := c.Do(req)
	if err != nil {
		// some Dahua/Amcrest cameras fail here because two simultaneous
		// backchannel connections
		//if c.Backchannel {
		//	c.Backchannel = false
		//	if err = c.Connect(); err != nil {
		//		return 0, err
		//	}
		//	return c.SetupMedia(media)
		//}

		return 0, err
	}

	if c.Session == "" {
		// Session: 7116520596809429228
		// Session: 216525287999;timeout=60
		if s := res.Header.Get("Session"); s != "" {
			if i := strings.IndexByte(s, ';'); i > 0 {
				c.Session = s[:i]
				if i = strings.Index(s, "timeout="); i > 0 {
					c.keepalive, _ = strconv.Atoi(s[i+8:])
				}
			} else {
				c.Session = s
			}
		}
	}

	// we send our `interleaved`, but camera can answer with another

	// Transport: RTP/AVP/TCP;unicast;interleaved=10-11;ssrc=10117CB7
	// Transport: RTP/AVP/TCP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0
	// Transport: RTP/AVP/TCP;ssrc=22345682;interleaved=0-1
	transport = res.Header.Get("Transport")
	if !strings.HasPrefix(transport, "RTP/AVP/TCP;") {
		// Escam Q6 has a bug:
		// Transport: RTP/AVP;unicast;destination=192.168.1.111;source=192.168.1.222;interleaved=0-1
		if !strings.Contains(transport, ";interleaved=") {
			return 0, fmt.Errorf("wrong transport: %s", transport)
		}
	}

	channel := core.Between(transport, "interleaved=", "-")
	i, err := strconv.Atoi(channel)
	if err != nil {
		return 0, err
	}

	return byte(i), nil
}

func (c *Client) Play() (err error) {
	return c.WriteRequest(&util.Request{Method: MethodPlay, URL: c.URL})
}

func (c *Client) Record() (err error) {
	return c.WriteRequest(&util.Request{Method: MethodRecord, URL: c.URL})
}

func (c *Client) Teardown() (err error) {
	// allow TEARDOWN from any state (ex. ANNOUNCE > SETUP)
	return c.WriteRequest(&util.Request{Method: MethodTeardown, URL: c.URL})
}

func (c *Client) Destroy() {
	_ = c.Teardown()
	c.NetConnection.Destroy()
}
