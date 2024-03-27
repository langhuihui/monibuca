package rtmp

import (
	"io"
	"net"

	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type RTMPPlugin struct {
	m7s.Plugin
	ChunkSize int `default:"1024"`
	KeepAlive bool
}

func (p *RTMPPlugin) OnInit() {

}

func (p *RTMPPlugin) OnStopPublish(puber *m7s.Publisher, err error) {

}

var _ = m7s.InstallPlugin[*RTMPPlugin](m7s.DefaultYaml(`tcp:
  listenaddr: :1935`))

func (p *RTMPPlugin) OnTCPConnect(conn *net.TCPConn) {
	defer conn.Close()
	logger := p.Logger.With("remote", conn.RemoteAddr().String())
	senders := make(map[uint32]*RTMPSender)
	receivers := make(map[uint32]*RTMPReceiver)
	var err error
	logger.Info("conn")
	defer func() {
		p.Info("conn close")
		for _, sender := range senders {
			sender.Stop(err)
		}
		for _, receiver := range receivers {
			receiver.Stop(err)
		}
	}()
	nc := NewNetConnection(conn)
	// ctx, cancel := context.WithCancel(p)
	// defer cancel()
	/* Handshake */
	if err = nc.Handshake(); err != nil {
		logger.Error("handshake", "error", err)
		return
	}
	var msg *Chunk
	var gstreamid uint32
	for {
		if msg, err = nc.RecvMessage(); err == nil {
			if msg.MessageLength <= 0 {
				continue
			}
			switch msg.MessageTypeID {
			case RTMP_MSG_AMF0_COMMAND:
				if msg.MsgData == nil {
					break
				}
				cmd := msg.MsgData.(Commander).GetCommand()
				logger.Debug("recv cmd", "commandName", cmd.CommandName, "streamID", msg.MessageStreamID)
				switch cmd := msg.MsgData.(type) {
				case *CallMessage: //connect
					app := cmd.Object["app"]                       // 客户端要连接到的服务应用名
					objectEncoding := cmd.Object["objectEncoding"] // AMF编码方法
					switch v := objectEncoding.(type) {
					case float64:
						nc.ObjectEncoding = v
					default:
						nc.ObjectEncoding = 0
					}
					nc.AppName = app.(string)
					logger.Info("connect", "appName", nc.AppName, "objectEncoding", nc.ObjectEncoding)
					err = nc.SendMessage(RTMP_MSG_ACK_SIZE, Uint32Message(512<<10))
					if err != nil {
						logger.Error("sendMessage ack size", "error", err)
						return
					}
					nc.WriteChunkSize = p.ChunkSize
					err = nc.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(p.ChunkSize))
					if err != nil {
						logger.Error("sendMessage chunk size", "error", err)
						return
					}
					err = nc.SendMessage(RTMP_MSG_BANDWIDTH, &SetPeerBandwidthMessage{
						AcknowledgementWindowsize: uint32(512 << 10),
						LimitType:                 byte(2),
					})
					if err != nil {
						logger.Error("sendMessage bandwidth", "error", err)
						return
					}
					err = nc.SendStreamID(RTMP_USER_STREAM_BEGIN, 0)
					if err != nil {
						logger.Error("sendMessage stream begin", "error", err)
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
						logger.Error("sendMessage connect", "error", err)
						return
					}
				case *CommandMessage: // "createStream"
					gstreamid++
					logger.Info("createStream:", "streamId", gstreamid)
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
					// 	if p, ok := s.Publisher.(*RTMPReceiver); ok {
					// 		// m.CommandName = "releaseStream_result"
					// 		p.Stop()
					// 		delete(receivers, p.StreamID)
					// 	}
					// }
					// err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
				case *PublishMessage:
					receiver := &RTMPReceiver{
						NetStream: NetStream{
							NetConnection: nc,
							StreamID:      cmd.StreamId,
						},
					}
					// receiver.SetParentCtx(ctx)
					if !p.KeepAlive {
						// receiver.SetIO(conn)
					}
					receiver.Publisher, err = p.Publish(nc.AppName + "/" + cmd.PublishingName)
					if err != nil {
						delete(receivers, cmd.StreamId)
						err = receiver.Response(cmd.TransactionId, NetStream_Publish_BadName, Level_Error)
					} else {
						receivers[cmd.StreamId] = receiver
						receiver.Begin()
						err = receiver.Response(cmd.TransactionId, NetStream_Publish_Start, Level_Status)
					}
				case *PlayMessage:
					streamPath := nc.AppName + "/" + cmd.StreamName
					sender := &RTMPSender{}
					sender.NetConnection = nc
					sender.StreamID = cmd.StreamId
					// sender.SetParentCtx(ctx)
					if !p.KeepAlive {
						// sender.SetIO(conn)
					}
					// sender.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), sender.StreamID)
					sender.Subscriber, err = p.Subscribe(streamPath)
					if err != nil {
						err = sender.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
					} else {
						senders[sender.StreamID] = sender
						sender.Begin()
						err = sender.Response(cmd.TransactionId, NetStream_Play_Reset, Level_Status)
						err = sender.Response(cmd.TransactionId, NetStream_Play_Start, Level_Status)
						sender.Init()
						go sender.Handle(sender.SendAudio, sender.SendVideo)
					}
					// if RTMPPlugin.Subscribe(streamPath, sender) != nil {
					// 	sender.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
					// } else {
					// 	senders[sender.StreamID] = sender
					// 	sender.Begin()
					// 	sender.Response(cmd.TransactionId, NetStream_Play_Reset, Level_Status)
					// 	sender.Response(cmd.TransactionId, NetStream_Play_Start, Level_Status)
					// 	go sender.PlayRaw()
					// }
				}
			case RTMP_MSG_AUDIO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.WriteAudio(&RTMPAudio{msg.AVData})
				} else {
					logger.Warn("ReceiveAudio", "MessageStreamID", msg.MessageStreamID)
				}
			case RTMP_MSG_VIDEO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.WriteVideo(&RTMPVideo{msg.AVData})
				} else {
					logger.Warn("ReceiveVideo", "MessageStreamID", msg.MessageStreamID)
				}
			}
		} else if err == io.EOF || err == io.ErrUnexpectedEOF {
			logger.Info("rtmp client closed")
			return
		} else {
			logger.Warn("ReadMessage", "error", err)
			return
		}
	}
}
