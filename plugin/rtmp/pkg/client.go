package rtmp

import (
	"crypto/tls"
	"errors"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"net"
	"net/url"
	"strings"

	"m7s.live/v5"
)

func (c *Client) Start() (err error) {
	var addr string
	if c.direction == DIRECTION_PULL {
		addr = c.pullCtx.Connection.RemoteURL
		err = c.pullCtx.Publish()
		if err != nil {
			return
		}
	} else {
		addr = c.pushCtx.Connection.RemoteURL
	}
	c.u, err = url.Parse(addr)
	if err != nil {
		return
	}
	ps := strings.Split(c.u.Path, "/")
	if len(ps) < 3 {
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
	if isRtmps {
		var tlsconn *tls.Conn
		tlsconn, err = tls.Dial("tcp", c.u.Host, &tls.Config{})
		conn = tlsconn
	} else {
		conn, err = net.Dial("tcp", c.u.Host)
	}
	if err != nil {
		return err
	}
	c.Init(conn)
	c.Logger = c.Logger.With("local", conn.LocalAddr().String())
	c.Info("connect")
	c.WriteChunkSize = c.chunkSize
	c.AppName = strings.Join(ps[1:len(ps)-1], "/")
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
	var msg *Chunk
	for err == nil {
		if msg, err = c.RecvMessage(); err != nil {
			return err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case Response_Result, Response_OnStatus:
				switch response := msg.MsgData.(type) {
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
						c.Receivers[response.StreamId] = c.pullCtx.Publisher
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
	}
	return
}
