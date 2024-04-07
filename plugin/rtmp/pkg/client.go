package rtmp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"

	"m7s.live/m7s/v5"
)

func NewRTMPClient(addr string, logger *slog.Logger, chunkSize int) (client *NetConnection, err error) {
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
	defer func() {
		if err != nil || client == nil {
			conn.Close()
		}
	}()
	client = NewNetConnection(conn)
	client.Logger = logger
	err = client.ClientHandshake()
	if err != nil {
		return nil, err
	}
	client.AppName = strings.Join(ps[1:len(ps)-1], "/")
	err = client.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(chunkSize))
	if err != nil {
		return
	}
	client.WriteChunkSize = chunkSize
	path := u.Path
	if len(u.Query()) != 0 {
		path += "?" + u.RawQuery
	}
	err = client.SendMessage(RTMP_MSG_AMF0_COMMAND, &CallMessage{
		CommandMessage{"connect", 1},
		map[string]any{
			"app":      client.AppName,
			"flashVer": "monibuca/" + m7s.Version,
			"swfUrl":   addr,
			"tcUrl":    strings.TrimSuffix(addr, path) + "/" + client.AppName,
		},
		nil,
	})
	for err != nil {
		msg, err := client.RecvMessage()
		if err != nil {
			return nil, err
		}
		switch msg.MessageTypeID {
		case RTMP_MSG_AMF0_COMMAND:
			cmd := msg.MsgData.(Commander).GetCommand()
			switch cmd.CommandName {
			case "_result":
				response := msg.MsgData.(*ResponseMessage)
				if response.Infomation["code"] == NetConnection_Connect_Success {
					return client, nil
				} else {
					return nil, err
				}
			default:
				fmt.Println(cmd.CommandName)
			}
		}
	}
	return
}
