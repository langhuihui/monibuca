package rtmp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"m7s.live/m7s/v5"
)

func createClient(c *m7s.Connection) (*NetStream, error) {
	chunkSize := 4096
	addr := c.RemoteURL

	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	ps := strings.Split(u.Path, "/")
	if len(ps) < 3 {
		return nil, errors.New("illegal rtmp url")
	}
	isRtmps := u.Scheme == "rtmps"
	if strings.Count(u.Host, ":") == 0 {
		if isRtmps {
			u.Host += ":443"
		} else {
			u.Host += ":1935"
		}
	}
	var conn net.Conn
	if isRtmps {
		var tlsconn *tls.Conn
		tlsconn, err = tls.Dial("tcp", u.Host, &tls.Config{})
		conn = tlsconn
	} else {
		conn, err = net.Dial("tcp", u.Host)
	}
	if err != nil {
		return nil, err
	}
	ns := &NetStream{}
	ns.NetConnection = NewNetConnection(conn)
	ns.Logger = c.Logger.With("local", conn.LocalAddr().String())
	c.Info("connect")
	defer func() {
		if err != nil {
			ns.Dispose()
		}
	}()
	if err = ns.ClientHandshake(); err != nil {
		return ns, err
	}
	ns.AppName = strings.Join(ps[1:len(ps)-1], "/")
	err = ns.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(chunkSize))
	if err != nil {
		return ns, err
	}
	ns.WriteChunkSize = chunkSize
	path := u.Path
	if len(u.Query()) != 0 {
		path += "?" + u.RawQuery
	}
	err = ns.SendMessage(RTMP_MSG_AMF0_COMMAND, &CallMessage{
		CommandMessage{"connect", 1},
		map[string]any{
			"app":      ns.AppName,
			"flashVer": "monibuca/" + m7s.Version,
			"swfUrl":   addr,
			"tcUrl":    strings.TrimSuffix(addr, path) + "/" + ns.AppName,
		},
		nil,
	})
	var msg *Chunk
	for err != nil {
		msg, err = ns.RecvMessage()
		if err != nil {
			return ns, err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case "_result":
				c.Description = msg.MsgData.(*ResponseMessage).Properties
				response := msg.MsgData.(*ResponseMessage)
				if response.Infomation["code"] == NetConnection_Connect_Success {
					err = ns.SendMessage(RTMP_MSG_AMF0_COMMAND, &CommandMessage{"createStream", 2})
					if err == nil {
						c.Info("connected")
					}
				}
				return ns, err
			default:
				fmt.Println(cmd.CommandName)
			}
		}
	}

	return ns, nil
}

func Pull(p *m7s.PullContext) (err error) {
	var connection *NetStream
	if connection, err = createClient(&p.Connection); err != nil {
		return
	}
	defer connection.Dispose()
	var msg *Chunk
	for err == nil {
		if msg, err = connection.RecvMessage(); err != nil {
			return err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AUDIO:
			if p.Publisher.PubAudio {
				err = p.Publisher.WriteAudio(msg.AVData.WrapAudio())
			} else {
				msg.AVData.Recycle()
			}
		case RTMP_MSG_VIDEO:
			if p.Publisher.PubVideo {
				err = p.Publisher.WriteVideo(msg.AVData.WrapVideo())
			} else {
				msg.AVData.Recycle()
			}
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case "_result":
				if response, ok := msg.MsgData.(*ResponseCreateStreamMessage); ok {
					connection.StreamID = response.StreamId
					m := &PlayMessage{}
					m.StreamId = response.StreamId
					m.TransactionId = 4
					m.CommandMessage.CommandName = "play"
					URL, _ := url.Parse(p.Connection.RemoteURL)
					ps := strings.Split(URL.Path, "/")
					args := URL.Query()
					m.StreamName = ps[len(ps)-1]
					if len(args) > 0 {
						m.StreamName += "?" + args.Encode()
					}
					connection.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
					// if response, ok := msg.MsgData.(*ResponsePlayMessage); ok {
					// 	if response.Object["code"] == "NetStream.Play.Start" {

					// 	} else if response.Object["level"] == Level_Error {
					// 		return errors.New(response.Object["code"].(string))
					// 	}
					// } else {
					// 	return errors.New("pull faild")
					// }
				}
			}
		}
	}
	return
}

func Push(p *m7s.PushContext) (err error) {
	var connection *NetStream
	if connection, err = createClient(&p.Connection); err != nil {
		return
	}
	defer connection.Dispose()
	var msg *Chunk
	for err == nil {
		msg, err = connection.RecvMessage()
		if err != nil {
			return err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case Response_Result, Response_OnStatus:
				if response, ok := msg.MsgData.(*ResponseCreateStreamMessage); ok {
					connection.StreamID = response.StreamId
					URL, _ := url.Parse(p.Connection.RemoteURL)
					_, streamPath, _ := strings.Cut(URL.Path, "/")
					_, streamPath, _ = strings.Cut(streamPath, "/")
					args := URL.Query()
					if len(args) > 0 {
						streamPath += "?" + args.Encode()
					}
					err = connection.SendMessage(RTMP_MSG_AMF0_COMMAND, &PublishMessage{
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
				} else if response, ok := msg.MsgData.(*ResponsePublishMessage); ok {
					if response.Infomation["code"] == NetStream_Publish_Start {
						audio, video := connection.CreateSender(true)
						go func() {
							for err == nil {
								msg, err = connection.RecvMessage()
							}
							p.Subscriber.Stop(err)
						}()
						return m7s.PlayBlock(p.Subscriber, audio.HandleAudio, video.HandleVideo)
					} else {
						return errors.New(response.Infomation["code"].(string))
					}
				}
			}
		}
	}
	return
}
