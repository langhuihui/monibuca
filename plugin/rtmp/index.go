package plugin_rtmp

import (
	"errors"
	"io"
	"maps"
	"net"
	"slices"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
	. "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type RTMPPlugin struct {
	pb.UnimplementedRtmpServer
	m7s.Plugin
	ChunkSize int `default:"1024"`
	KeepAlive bool
	C2        bool
}

var _ = m7s.InstallPlugin[RTMPPlugin](m7s.DefaultYaml(`tcp:
  listenaddr: :1935`), &pb.Rtmp_ServiceDesc, pb.RegisterRtmpHandler, NewPusher, NewPuller)

func (p *RTMPPlugin) OnInit() error {
	for streamPath, url := range p.GetCommonConf().PullOnStart {
		p.Pull(streamPath, url)
	}
	return nil
}

func (p *RTMPPlugin) GetPullableList() []string {
	return slices.Collect(maps.Keys(p.GetCommonConf().PullOnSub))
}

func (p *RTMPPlugin) OnTCPConnect(conn *net.TCPConn) {
	var err error
	nc := NewNetConnection(conn)
	nc.Logger = p.With("remote", conn.RemoteAddr().String())
	p.AddTask(nc).WaitStarted()
	defer func() {
		nc.Stop(err)
	}()
	/* Handshake */
	if err = nc.Handshake(p.C2); err != nil {
		nc.Error("handshake", "error", err)
		return
	}
	var msg *Chunk
	var gstreamid uint32
	for err == nil {
		if msg, err = nc.RecvMessage(); err == nil {
			if msg.MessageLength <= 0 {
				continue
			}
			switch msg.MessageTypeID {
			case RTMP_MSG_AMF0_COMMAND:
				if msg.MsgData == nil {
					err = errors.New("msg.MsgData is nil")
					break
				}
				cmd := msg.MsgData.(Commander).GetCommand()
				nc.Debug("recv cmd", "commandName", cmd.CommandName, "streamID", msg.MessageStreamID)
				switch cmd := msg.MsgData.(type) {
				case *CallMessage: //connect
					nc.Description = cmd.Object
					app := cmd.Object["app"]                       // 客户端要连接到的服务应用名
					objectEncoding := cmd.Object["objectEncoding"] // AMF编码方法
					switch v := objectEncoding.(type) {
					case float64:
						nc.ObjectEncoding = v
					default:
						nc.ObjectEncoding = 0
					}
					nc.AppName = app.(string)
					nc.Info("connect", "appName", nc.AppName, "objectEncoding", nc.ObjectEncoding)
					err = nc.SendMessage(RTMP_MSG_ACK_SIZE, Uint32Message(512<<10))
					if err != nil {
						nc.Error("sendMessage ack size", "error", err)
						return
					}
					nc.WriteChunkSize = p.ChunkSize
					err = nc.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(p.ChunkSize))
					if err != nil {
						nc.Error("sendMessage chunk size", "error", err)
						return
					}
					err = nc.SendMessage(RTMP_MSG_BANDWIDTH, &SetPeerBandwidthMessage{
						AcknowledgementWindowsize: uint32(512 << 10),
						LimitType:                 byte(2),
					})
					if err != nil {
						nc.Error("sendMessage bandwidth", "error", err)
						return
					}
					err = nc.SendStreamID(RTMP_USER_STREAM_BEGIN, 0)
					if err != nil {
						nc.Error("sendMessage stream begin", "error", err)
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
						"objectEncoding": nc.ObjectEncoding,
					}
					err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
					if err != nil {
						nc.Error("sendMessage connect", "error", err)
					}
				case *CommandMessage: // "createStream"
					gstreamid++
					nc.Info("createStream:", "streamId", gstreamid)
					nc.ResponseCreateStream(cmd.TransactionId, gstreamid)
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
					// 		// m.CommandName = "releaseStream_result"
					// 		p.Stop()
					// 		delete(receivers, p.StreamID)
					// 	}
					// }
					// err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
				case *PublishMessage:
					ns := NetStream{
						NetConnection: nc,
						StreamID:      cmd.StreamId,
					}
					var publisher *m7s.Publisher
					publisher, err = p.Publish(nc.Context, nc.AppName+"/"+cmd.PublishingName)
					if err != nil {
						err = ns.Response(cmd.TransactionId, NetStream_Publish_BadName, Level_Error)
					} else {
						ns.Receivers[cmd.StreamId] = publisher
						err = ns.BeginPublish(cmd.TransactionId)
					}
					if err != nil {
						nc.Error("sendMessage publish", "error", err)
					} else {
						publisher.OnDispose(func() {
							nc.Stop(publisher.StopReason())
						})
					}
				case *PlayMessage:
					streamPath := nc.AppName + "/" + cmd.StreamName
					ns := NetStream{
						NetConnection: nc,
						StreamID:      cmd.StreamId,
					}
					var suber *m7s.Subscriber
					// sender.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), sender.StreamID)
					suber, err = p.Subscribe(nc.Context, streamPath)
					if err != nil {
						err = ns.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
					} else {
						err = ns.BeginPlay(cmd.TransactionId)
						ns.Subscribe(suber)
					}
					if err != nil {
						nc.Error("sendMessage play", "error", err)
					}
				}
			}
		} else if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			nc.Info("rtmp client closed")
		} else {
			nc.Warn("ReadMessage", "error", err)
		}
	}
}
