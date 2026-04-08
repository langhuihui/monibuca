package rtmp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	task "github.com/langhuihui/gotask"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"

	"m7s.live/v5"
)

// Fixed progress steps for RTMP pull workflow
var rtmpPullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing stream"},
	{Name: pkg.StepURLParsing, Description: "Parsing RTMP URL"},
	{Name: pkg.StepConnection, Description: "Connecting to RTMP server"},
	{Name: pkg.StepHandshake, Description: "Performing RTMP handshake"},
	{Name: pkg.StepStreaming, Description: "Receiving media stream"},
}

type Client struct {
	NetStream
	chunkSize int
	u         *url.URL
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return nil
}

func (c *Client) GetPushJob() *m7s.PushJob {
	return nil
}

const dialTimeout = 30 * time.Second

func (c *Client) commonStart(ctx context.Context, addr string) (err error) {
	c.u, err = url.Parse(addr)
	if err != nil {
		return
	}
	ps := strings.Split(c.u.Path, "/")
	if len(ps) < 2 {
		return errors.New("illegal rtmp url")
	}

	isRtmps := c.u.Scheme == "rtmps"
	if strings.Count(c.u.Host, ":") == 0 {
		if isRtmps {
			c.u.Host += ":443"
		} else {
			c.u.Host += ":1935"
		}
	}
	var conn net.Conn
	dialer := &net.Dialer{Timeout: dialTimeout}
	if isRtmps {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config:    &tls.Config{InsecureSkipVerify: true},
		}
		conn, err = tlsDialer.DialContext(ctx, "tcp", c.u.Host)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", c.u.Host)
	}
	if err != nil {
		return err
	}

	c.Init(conn)
	c.SetDescription("local", conn.LocalAddr().String())
	c.Info("connect")
	c.WriteChunkSize = c.chunkSize
	c.AppName = strings.Join(ps[1:len(ps)-1], "/")

	return err
}

func (c *Client) commonRun(handler func(commander Commander) error) (err error) {
	if err = c.ClientHandshake(); err != nil {
		return
	}
	err = c.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(c.chunkSize))
	if err != nil {
		return
	}
	path := c.u.Path
	if len(c.u.Query()) != 0 {
		path += "?" + c.u.RawQuery
	}
	err = c.SendMessage(RTMP_MSG_AMF0_COMMAND, &CallMessage{
		CommandMessage{"connect", 1},
		map[string]any{
			"app":      c.AppName,
			"flashVer": "monibuca/" + m7s.Version,
			"swfUrl":   c.u.String(),
			"tcUrl":    strings.TrimSuffix(c.u.String(), path) + "/" + c.AppName,
		},
		nil,
	})
	var commander Commander
	for err == nil {
		if commander, err = c.RecvMessage(); err != nil {
			return err
		}
		cmd := commander.GetCommand()
		c.Debug(cmd.CommandName)
		switch cmd.CommandName {
		case Response_Result, Response_OnStatus:
			switch response := commander.(type) {
			case *ResponseMessage:
				c.SetDescriptions(response.Properties)
				if response.Infomation["code"] == NetConnection_Connect_Success {
					err = c.SendMessage(RTMP_MSG_AMF0_COMMAND, &CommandMessage{"createStream", 2})
					if err == nil {
						c.Info("connected")
						c.OnConnected()
					}
				}
			case *ResponseCreateStreamMessage:
				c.StreamID = response.StreamId
				if handler != nil {
					if err = handler(commander); err != nil {
						return err
					}
				}
			}
		}
	}
	return
}

type Puller struct {
	Client
	pullCtx m7s.PullJob
}

func (p *Puller) GetPullJob() *m7s.PullJob {
	return &p.pullCtx
}

func (p *Puller) Start() (err error) {
	// Initialize progress tracking for pull operations
	p.pullCtx.SetProgressStepsDefs(rtmpPullSteps)

	addr := p.pullCtx.Connection.RemoteURL
	err = p.pullCtx.Publish()
	if err != nil {
		p.pullCtx.Fail(err.Error())
		return
	}

	p.pullCtx.GoToStepConst(pkg.StepURLParsing)

	err = p.commonStart(p.pullCtx.Context, addr)
	if err != nil {
		p.pullCtx.Fail(err.Error())
		return
	}

	p.pullCtx.GoToStepConst(pkg.StepConnection)
	p.pullCtx.GoToStepConst(pkg.StepHandshake)
	p.pullCtx.GoToStepConst(pkg.StepStreaming)

	return
}

func (p *Puller) Run() (err error) {
	return p.commonRun(func(commander Commander) error {
		switch response := commander.(type) {
		case *ResponseCreateStreamMessage:
			p.StreamID = response.StreamId
			m := &PlayMessage{}
			m.StreamId = response.StreamId
			m.TransactionId = 4
			m.CommandMessage.CommandName = "play"
			URL, _ := url.Parse(p.pullCtx.Connection.RemoteURL)
			ps := strings.Split(URL.Path, "/")
			args := URL.Query()
			m.StreamName = ps[len(ps)-1]
			if len(args) > 0 {
				m.StreamName += "?" + args.Encode()
			}
			if p.pullCtx.Publisher != nil {
				p.Writers[response.StreamId] = &struct {
					m7s.PublishWriter[*AudioFrame, *VideoFrame]
					*m7s.Publisher
				}{Publisher: p.pullCtx.Publisher}
			}
			return p.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
		}
		return nil
	})
}

type Pusher struct {
	Client
	pushCtx m7s.PushJob
}

func (p *Pusher) GetPushJob() *m7s.PushJob {
	return &p.pushCtx
}

func (p *Pusher) Start() (err error) {
	return p.commonStart(p.pushCtx.Context, p.pushCtx.Connection.RemoteURL)
}

func (p *Pusher) Run() (err error) {
	return p.commonRun(func(commander Commander) error {
		switch response := commander.(type) {
		case *ResponseCreateStreamMessage:
			p.StreamID = response.StreamId
			err = p.pushCtx.Subscribe()
			if err != nil {
				return err
			}
			URL, _ := url.Parse(p.pushCtx.Connection.RemoteURL)
			_, streamPath, _ := strings.Cut(URL.Path, "/")
			_, streamPath, _ = strings.Cut(streamPath, "/")
			args := URL.Query()
			if len(args) > 0 {
				streamPath += "?" + args.Encode()
			}
			return p.SendMessage(RTMP_MSG_AMF0_COMMAND, &PublishMessage{
				CURDStreamMessage{
					CommandMessage{
						"publish",
						1,
					},
					response.StreamId,
				},
				streamPath,
				"live",
			})
		case *ResponsePublishMessage:
			if response.Infomation["code"] == NetStream_Publish_Start {
				p.Subscribe(p.pushCtx.Subscriber)
			} else {
				return errors.New(response.Infomation["code"].(string))
			}
		}
		return nil
	})
}

func NewPuller(_ config.Pull) m7s.IPuller {
	ret := &Puller{
		Client: Client{
			chunkSize: 4096,
		},
	}
	ret.NetConnection = &NetConnection{}
	ret.SetDescription(task.OwnerTypeKey, "RTMPPuller")
	return ret
}

func NewPusher() m7s.IPusher {
	ret := &Pusher{
		Client: Client{
			chunkSize: 4096,
		},
	}
	ret.NetConnection = &NetConnection{}
	ret.SetDescription(task.OwnerTypeKey, "RTMPPusher")
	return ret
}
