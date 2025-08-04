package rtmp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"

	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"

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

func (c *Client) Start() (err error) {
	var addr string
	if c.direction == DIRECTION_PULL {
		// Initialize progress tracking for pull operations
		c.pullCtx.SetProgressStepsDefs(rtmpPullSteps)

		addr = c.pullCtx.Connection.RemoteURL
		err = c.pullCtx.Publish()
		if err != nil {
			c.pullCtx.Fail(err.Error())
			return
		}

		c.pullCtx.GoToStepConst(pkg.StepURLParsing)
	} else {
		addr = c.pushCtx.Connection.RemoteURL
	}
	c.u, err = url.Parse(addr)
	if err != nil {
		if c.direction == DIRECTION_PULL {
			c.pullCtx.Fail(err.Error())
		}
		return
	}
	ps := strings.Split(c.u.Path, "/")
	if len(ps) < 2 {
		if c.direction == DIRECTION_PULL {
			c.pullCtx.Fail("illegal rtmp url")
		}
		return errors.New("illegal rtmp url")
	}

	if c.direction == DIRECTION_PULL {
		c.pullCtx.GoToStepConst(pkg.StepConnection)
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
	if isRtmps {
		var tlsconn *tls.Conn
		tlsconn, err = tls.Dial("tcp", c.u.Host, &tls.Config{
			InsecureSkipVerify: true,
		})
		conn = tlsconn
	} else {
		conn, err = net.Dial("tcp", c.u.Host)
	}
	if err != nil {
		if c.direction == DIRECTION_PULL {
			c.pullCtx.Fail(err.Error())
		}
		return err
	}

	if c.direction == DIRECTION_PULL {
		c.pullCtx.GoToStepConst(pkg.StepHandshake)
	}

	c.Init(conn)
	c.SetDescription("local", conn.LocalAddr().String())
	c.Info("connect")
	c.WriteChunkSize = c.chunkSize
	c.AppName = strings.Join(ps[1:len(ps)-1], "/")

	if c.direction == DIRECTION_PULL {
		c.pullCtx.GoToStepConst(pkg.StepStreaming)
	}

	return err
}

const (
	DIRECTION_PULL = "pull"
	DIRECTION_PUSH = "push"
)

type Client struct {
	NetStream
	chunkSize int
	pullCtx   m7s.PullJob
	pushCtx   m7s.PushJob
	direction string
	u         *url.URL
}

func (c *Client) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func (c *Client) GetPushJob() *m7s.PushJob {
	return &c.pushCtx
}

func NewPuller(_ config.Pull) m7s.IPuller {
	ret := &Client{
		direction: DIRECTION_PULL,
		chunkSize: 4096,
	}
	ret.NetConnection = &NetConnection{}
	ret.SetDescription(task.OwnerTypeKey, "RTMPPuller")
	return ret
}

func NewPusher() m7s.IPusher {
	ret := &Client{
		direction: DIRECTION_PUSH,
		chunkSize: 4096,
	}
	ret.NetConnection = &NetConnection{}
	ret.SetDescription(task.OwnerTypeKey, "RTMPPusher")
	return ret
}

func (c *Client) Run() (err error) {
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
		switch cmd.CommandName {
		case Response_Result, Response_OnStatus:
			switch response := commander.(type) {
			case *ResponseMessage:
				c.SetDescriptions(response.Properties)
				if response.Infomation["code"] == NetConnection_Connect_Success {
					err = c.SendMessage(RTMP_MSG_AMF0_COMMAND, &CommandMessage{"createStream", 2})
					if err == nil {
						c.Info("connected")
					}
				}
			case *ResponseCreateStreamMessage:
				c.StreamID = response.StreamId
				if c.direction == DIRECTION_PULL {
					m := &PlayMessage{}
					m.StreamId = response.StreamId
					m.TransactionId = 4
					m.CommandMessage.CommandName = "play"
					URL, _ := url.Parse(c.pullCtx.Connection.RemoteURL)
					ps := strings.Split(URL.Path, "/")
					args := URL.Query()
					m.StreamName = ps[len(ps)-1]
					if len(args) > 0 {
						m.StreamName += "?" + args.Encode()
					}
					if c.pullCtx.Publisher != nil {
						c.Writers[response.StreamId] = &struct {
							m7s.PublishWriter[*AudioFrame, *VideoFrame]
							*m7s.Publisher
						}{Publisher: c.pullCtx.Publisher}
					}
					err = c.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
					// if response, ok := msg.MsgData.(*ResponsePlayMessage); ok {
					// 	if response.Object["code"] == "NetStream.Play.Start" {

					// 	} else if response.Object["level"] == Level_Error {
					// 		return errors.New(response.Object["code"].(string))
					// 	}
					// } else {
					// 	return errors.New("pull faild")
					// }
				} else {
					err = c.pushCtx.Subscribe()
					if err != nil {
						return
					}
					URL, _ := url.Parse(c.pushCtx.Connection.RemoteURL)
					_, streamPath, _ := strings.Cut(URL.Path, "/")
					_, streamPath, _ = strings.Cut(streamPath, "/")
					args := URL.Query()
					if len(args) > 0 {
						streamPath += "?" + args.Encode()
					}
					err = c.SendMessage(RTMP_MSG_AMF0_COMMAND, &PublishMessage{
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
				}
			case *ResponsePublishMessage:
				if response.Infomation["code"] == NetStream_Publish_Start {
					c.Subscribe(c.pushCtx.Subscriber)
				} else {
					return errors.New(response.Infomation["code"].(string))
				}
			}
		}
	}
	return
}
