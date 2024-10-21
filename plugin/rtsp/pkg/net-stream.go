package rtsp

import (
	"errors"
	"fmt"
	"m7s.live/v5/pkg/util"
	"net/http"
	"strconv"
	"strings"
)

type Stream struct {
	*NetConnection
	AudioChannelID int
	VideoChannelID int
}

func (c *Stream) Do(req *util.Request) (*util.Response, error) {
	if err := c.WriteRequest(req); err != nil {
		return nil, err
	}

	res, err := c.ReadResponse()
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized {
		switch c.auth.Method {
		case util.AuthNone:
			if c.auth.ReadNone(res) {
				return c.Do(req)
			}
			return nil, errors.New("user/pass not provided")
		case util.AuthUnknown:
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

func (c *Stream) Options() error {
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

func (c *Stream) Describe() (medias []*Media, err error) {
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
		clone := make([]*Media, 0, len(medias))
		for _, media := range medias {
			if strings.Contains(c.Media, media.Kind) {
				clone = append(clone, media)
			}
		}
		medias = clone
	}

	return
}

func (c *Stream) Announce(medias []*Media) (err error) {
	req := &util.Request{
		Method: MethodAnnounce,
		URL:    c.URL,
		Header: map[string][]string{
			"Content-Type": {"application/sdp"},
		},
	}

	req.Body, err = MarshalSDP(c.SessionName, medias)
	if err != nil {
		return err
	}

	_, err = c.Do(req)

	return
}

func (c *Stream) SetupMedia(media *Media, index int) (byte, error) {
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

	channel := Between(transport, "interleaved=", "-")
	i, err := strconv.Atoi(channel)
	if err != nil {
		return 0, err
	}

	return byte(i), nil
}

func (c *Stream) Play() (err error) {
	return c.WriteRequest(&util.Request{Method: MethodPlay, URL: c.URL})
}

func (c *Stream) Record() (err error) {
	return c.WriteRequest(&util.Request{Method: MethodRecord, URL: c.URL})
}

func (c *Stream) Teardown() (err error) {
	// allow TEARDOWN from any state (ex. ANNOUNCE > SETUP)
	return c.WriteRequest(&util.Request{Method: MethodTeardown, URL: c.URL})
}

//func (ns *Stream) Dispose() {
//	//_ = ns.Teardown()
//	ns.NetConnection.Dispose()
//}
