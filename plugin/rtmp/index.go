package plugin_rtmp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/rtmp/pb"
	. "m7s.live/v5/plugin/rtmp/pkg"
)

type RTMPPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	ChunkSize int `default:"1024"`
	KeepAlive bool
	C2        bool
}

var _ = m7s.InstallPlugin[RTMPPlugin](m7s.PluginMeta{
	DefaultYaml: `tcp:
  listenaddr: :1935`,
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
	NewPusher:           NewPusher,
	NewPuller:           NewPuller,
	NewPullProxy:        NewPullProxy,
	NewPushProxy:        NewPushProxy,
})

type RTMPServer struct {
	NetConnection
	conf *RTMPPlugin
}

func (p *RTMPPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
	ret := &RTMPServer{conf: p}
	ret.Init(conn)
	ret.Logger = p.Logger.With("remote", conn.RemoteAddr().String())
	return ret
}

func (task *RTMPServer) Go() (err error) {
	if err = task.Handshake(task.conf.C2); err != nil {
		task.Error("handshake", "error", err)
		return
	}
	var commander Commander
	var gstreamid uint32
	for err == nil {
		if commander, err = task.RecvMessage(); err == nil {
			task.Debug("recv cmd", "commandName", commander.GetCommand().CommandName)
			switch cmd := commander.(type) {
			case *CallMessage: //connect
				task.SetDescriptions(cmd.Object)
				app := cmd.Object["app"]                       // 客户端要连接到的服务应用名
				objectEncoding := cmd.Object["objectEncoding"] // AMF编码方法
				switch v := objectEncoding.(type) {
				case float64:
					task.ObjectEncoding = v
				default:
					task.ObjectEncoding = 0
				}
				task.AppName = app.(string)
				task.Info("connect", "appName", task.AppName, "objectEncoding", task.ObjectEncoding)
				err = task.SendMessage(RTMP_MSG_ACK_SIZE, Uint32Message(512<<10))
				if err != nil {
					task.Error("sendMessage ack size", "error", err)
					return
				}
				task.WriteChunkSize = task.conf.ChunkSize
				err = task.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(task.conf.ChunkSize))
				if err != nil {
					task.Error("sendMessage chunk size", "error", err)
					return
				}
				err = task.SendMessage(RTMP_MSG_BANDWIDTH, &SetPeerBandwidthMessage{
					AcknowledgementWindowsize: uint32(512 << 10),
					LimitType:                 byte(2),
				})
				if err != nil {
					task.Error("sendMessage bandwidth", "error", err)
					return
				}
				err = task.SendStreamID(RTMP_USER_STREAM_BEGIN, 0)
				if err != nil {
					task.Error("sendMessage stream begin", "error", err)
					return
				}
				m := new(ResponseConnectMessage)
				m.CommandName = Response_Result
				m.TransactionId = 1
				m.Properties = map[string]any{
					"fmsVer":       "monibuca/" + m7s.Version,
					"capabilities": 31,
					"mode":         1,
					"Author":       "dexter",
				}
				m.Infomation = map[string]any{
					"level":          Level_Status,
					"code":           NetConnection_Connect_Success,
					"objectEncoding": task.ObjectEncoding,
				}
				err = task.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
				if err != nil {
					task.Error("sendMessage connect", "error", err)
				}
			case *CommandMessage: // "createStream"
				gstreamid++
				task.Info("createStream:", "streamId", gstreamid)
				task.ResponseCreateStream(cmd.TransactionId, gstreamid)
			case *CURDStreamMessage:
				// if stream, ok := receivers[cmd.StreamId]; ok {
				// 	stream.Stop()
				// 	delete(senders, cmd.StreamId)
				// }
			case *ReleaseStreamMessage:
				// m := &CommandMessage{
				// 	CommandName:   "releaseStream_error",
				// 	TransactionId: cmd.TransactionId,
				// }
				// s := engine.Streams.Get(nc.appName + "/" + cmd.StreamName)
				// if s != nil && s.Publisher != nil {
				// 	if p, ok := s.Publisher.(*Receiver); ok {
				// 		// m.CommandName = "releaseStream_result"				// 		p.Stop()
				// 		delete(receivers, p.StreamID)
				// 	}
				// }
				// err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
			case *PublishMessage:
				ns := NetStream{
					NetConnection: &task.NetConnection,
					StreamID:      cmd.StreamId,
				}
				var publisher *m7s.Publisher
				publisher, err = task.conf.Publish(task.Context, task.AppName+"/"+cmd.PublishingName)
				if err != nil {
					err = ns.Response(cmd.TransactionId, NetStream_Publish_BadName, Level_Error)
				} else {
					ns.Writers[cmd.StreamId] = &struct {
						m7s.PublishWriter[*AudioFrame, *VideoFrame]
						*m7s.Publisher
					}{Publisher: publisher}
					publisher.RemoteAddr = ns.RemoteAddr().String()
					err = ns.BeginPublish(cmd.TransactionId)
				}
				if err != nil {
					task.Error("sendMessage publish", "error", err)
				} else {
					publisher.Using(task)
				}
			case *PlayMessage:
				streamPath := task.AppName + "/" + cmd.StreamName
				ns := NetStream{
					NetConnection: &task.NetConnection,
					StreamID:      cmd.StreamId,
				}
				var suber *m7s.Subscriber
				suber, err = task.conf.Subscribe(task.Context, streamPath)
				if err != nil {
					err = ns.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
				} else {
					suber.RemoteAddr = ns.RemoteAddr().String()
					err = ns.BeginPlay(cmd.TransactionId)
					ns.Subscribe(suber)
				}
				if err != nil {
					task.Error("sendMessage play", "error", err)
				}
			}
		} else if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			task.Info("rtmp client closed", "error", err)
		} else {
			task.Warn("ReadMessage", "error", err)
		}
	}
	return
}

func (p *RTMPPlugin) Start() (err error) {
	if tcpAddr := p.GetCommonConf().TCP.ListenAddr; tcpAddr != "" {
		_, port, _ := strings.Cut(tcpAddr, ":")
		if port == "1935" {
			p.PushAddr = append(p.PushAddr, "rtmp://{hostName}/{streamPath}")
			p.PlayAddr = append(p.PlayAddr, "rtmp://{hostName}/{streamPath}")
		} else {
			p.PushAddr = append(p.PushAddr, fmt.Sprintf("rtmp://{hostName}:%s/{streamPath}", port))
			p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("rtmp://{hostName}:%s/{streamPath}", port))
		}
	}
	if tcpAddrTLS := p.GetCommonConf().TCP.ListenAddrTLS; tcpAddrTLS != "" {
		_, port, _ := strings.Cut(tcpAddrTLS, ":")
		if port == "443" {
			p.PushAddr = append(p.PushAddr, "rtmps://{hostName}/{streamPath}")
			p.PlayAddr = append(p.PlayAddr, "rtmps://{hostName}/{streamPath}")
		} else {
			p.PushAddr = append(p.PushAddr, fmt.Sprintf("rtmps://{hostName}:%s/{streamPath}", port))
			p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("rtmps://{hostName}:%s/{streamPath}", port))
		}
	}
	return
}
