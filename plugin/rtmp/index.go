package rtmp

import (
	"io"
	"net"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type RTMPPlugin struct {
	m7s.Plugin
	ChunkSize int
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
	// logger := RTMPPlugin.Logger.With(zap.String("remote", conn.RemoteAddr().String()))
	// senders := make(map[uint32]*RTMPSubscriber)
	receivers := make(map[uint32]*pkg.RTMPReceiver)
	var err error
	// logger.Info("conn")
	defer func() {
		// ze := zap.Error(err)
		// logger.Info("conn close", ze)
		// for _, sender := range senders {
		// sender.Stop(ze)
		// }
		// for _, receiver := range receivers {
		// receiver.Stop(ze)
		// }
	}()
	nc := pkg.NewNetConnection(conn)
	// ctx, cancel := context.WithCancel(p)
	// defer cancel()
	/* Handshake */
	if err = nc.Handshake(); err != nil {
		// logger.Error("handshake", zap.Error(err))
		return
	}
	var msg *pkg.Chunk
	var gstreamid uint32
	for {
		if msg, err = nc.RecvMessage(); err == nil {
			if msg.MessageLength <= 0 {
				continue
			}
			switch msg.MessageTypeID {
			case pkg.RTMP_MSG_AMF0_COMMAND:
				if msg.MsgData == nil {
					break
				}
				// cmd := msg.MsgData.(pkg.Commander).GetCommand()
				// logger.Debug("recv cmd", zap.String("commandName", cmd.CommandName), zap.Uint32("streamID", msg.MessageStreamID))
				switch cmd := msg.MsgData.(type) {
				case *pkg.CallMessage: //connect
					app := cmd.Object["app"]                       // 客户端要连接到的服务应用名
					objectEncoding := cmd.Object["objectEncoding"] // AMF编码方法
					switch v := objectEncoding.(type) {
					case float64:
						nc.ObjectEncoding = v
					default:
						nc.ObjectEncoding = 0
					}
					nc.AppName = app.(string)
					// logger.Info("connect", zap.String("appName", nc.appName), zap.Float64("objectEncoding", nc.objectEncoding))
					err = nc.SendMessage(pkg.RTMP_MSG_ACK_SIZE, pkg.Uint32Message(512<<10))
					nc.WriteChunkSize = p.ChunkSize
					err = nc.SendMessage(pkg.RTMP_MSG_CHUNK_SIZE, pkg.Uint32Message(p.ChunkSize))
					err = nc.SendMessage(pkg.RTMP_MSG_BANDWIDTH, &pkg.SetPeerBandwidthMessage{
						AcknowledgementWindowsize: uint32(512 << 10),
						LimitType:                 byte(2),
					})
					err = nc.SendStreamID(pkg.RTMP_USER_STREAM_BEGIN, 0)
					m := new(pkg.ResponseConnectMessage)
					m.CommandName = pkg.Response_Result
					m.TransactionId = 1
					m.Properties = map[string]any{
						"fmsVer":       "monibuca/" + m7s.Version,
						"capabilities": 31,
						"mode":         1,
						"Author":       "dexter",
					}
					m.Infomation = map[string]any{
						"level":          pkg.Level_Status,
						"code":           pkg.NetConnection_Connect_Success,
						"objectEncoding": nc.ObjectEncoding,
					}
					err = nc.SendMessage(pkg.RTMP_MSG_AMF0_COMMAND, m)
				case *pkg.CommandMessage: // "createStream"
					gstreamid++
					// logger.Info("createStream:", zap.Uint32("streamId", gstreamid))
					nc.ResponseCreateStream(cmd.TransactionId, gstreamid)
				case *pkg.CURDStreamMessage:
					// if stream, ok := receivers[cmd.StreamId]; ok {
					// 	stream.Stop()
					// 	delete(senders, cmd.StreamId)
					// }
				case *pkg.ReleaseStreamMessage:
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
				case *pkg.PublishMessage:
					receiver := &pkg.RTMPReceiver{
						NetStream: pkg.NetStream{
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
						err = receiver.Response(cmd.TransactionId, pkg.NetStream_Publish_BadName, pkg.Level_Error)
					} else {
						receivers[cmd.StreamId] = receiver
						receiver.Begin()
						err = receiver.Response(cmd.TransactionId, pkg.NetStream_Publish_Start, pkg.Level_Status)
					}
				case *pkg.PlayMessage:
					// streamPath := nc.appName + "/" + cmd.StreamName
					// sender := &RTMPSubscriber{}
					// sender.NetStream = NetStream{
					// 	nc,
					// 	cmd.StreamId,
					// }
					// sender.SetParentCtx(ctx)
					// if !config.KeepAlive {
					// 	sender.SetIO(conn)
					// }
					// sender.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), sender.StreamID)
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
			case pkg.RTMP_MSG_AUDIO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.ReceiveAudio(msg)
				} else {
					// logger.Warn("ReceiveAudio", zap.Uint32("MessageStreamID", msg.MessageStreamID))
				}
			case pkg.RTMP_MSG_VIDEO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.ReceiveVideo(msg)
				} else {
					// logger.Warn("ReceiveVideo", zap.Uint32("MessageStreamID", msg.MessageStreamID))
				}
			}
		} else if err == io.EOF || err == io.ErrUnexpectedEOF {
			// logger.Info("rtmp client closed")
			return
		} else {
			// logger.Warn("ReadMessage", zap.Error(err))
			return
		}
	}
}
