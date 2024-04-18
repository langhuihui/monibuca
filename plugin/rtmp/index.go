package plugin_rtmp

import (
	"context"
	"io"
	"net"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
	. "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type RTMPPlugin struct {
	pb.UnimplementedRtmpServer
	m7s.Plugin
	ChunkSize int `default:"1024"`
	KeepAlive bool
}

var _ = m7s.InstallPlugin[RTMPPlugin](m7s.DefaultYaml(`tcp:
  listenaddr: :1935`), &pb.Rtmp_ServiceDesc, pb.RegisterRtmpHandler)

func (p *RTMPPlugin) OnInit() error {
	for streamPath, url := range p.GetCommonConf().PullOnStart {
		go p.Pull(streamPath, url, &Client{})
	}
	return nil
}

func (p *RTMPPlugin) OnPull(puller *m7s.Puller) {
	p.OnPublish(&puller.Publisher)
}

func (p *RTMPPlugin) OnPublish(puber *m7s.Publisher) {
	if remoteURL, ok := p.GetCommonConf().PushList[puber.StreamPath]; ok {
		go p.Push(puber.StreamPath, remoteURL, &Client{})
	}
}

func (p *RTMPPlugin) OnTCPConnect(conn *net.TCPConn) {
	defer conn.Close()
	logger := p.Logger.With("remote", conn.RemoteAddr().String())
	receivers := make(map[uint32]*RTMPReceiver)
	var err error
	logger.Info("conn")
	nc := NewNetConnection(conn, logger)
	ctx, cancel := context.WithCancelCause(p)
	defer func() {
		logger.Info("conn close")
		cancel(err)
	}()
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
					receiver.Publisher, err = p.Publish(nc.AppName+"/"+cmd.PublishingName, ctx, conn)
					if err != nil {
						delete(receivers, cmd.StreamId)
						err = receiver.Response(cmd.TransactionId, NetStream_Publish_BadName, Level_Error)
					} else {
						receivers[cmd.StreamId] = receiver
						err = receiver.BeginPublish(cmd.TransactionId)
					}
					if err != nil {
						logger.Error("sendMessage publish", "error", err)
						return
					}
				case *PlayMessage:
					streamPath := nc.AppName + "/" + cmd.StreamName
					ns := NetStream{
						NetConnection: nc,
						StreamID:      cmd.StreamId,
					}
					var suber *m7s.Subscriber
					// sender.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), sender.StreamID)
					suber, err = p.Subscribe(streamPath, ctx, conn)
					if err != nil {
						err = ns.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
					} else {
						ns.BeginPlay(cmd.TransactionId)
						audio, video := ns.CreateSender()
						go suber.Handle(m7s.SubscriberHandler{
							OnAudio: func(a *RTMPAudio) error {
								return audio.SendFrame(&a.RTMPData)
							}, OnVideo: func(v *RTMPVideo) error {
								return video.SendFrame(&v.RTMPData)
							}})
					}
					if err != nil {
						logger.Error("sendMessage play", "error", err)
						return
					}
				}
			case RTMP_MSG_AUDIO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.WriteAudio(&RTMPAudio{msg.AVData})
					msg.AVData = RTMPData{}
					msg.AVData.ScalableMemoryAllocator = nc.ReadPool
				} else {
					logger.Warn("ReceiveAudio", "MessageStreamID", msg.MessageStreamID)
				}
			case RTMP_MSG_VIDEO:
				if r, ok := receivers[msg.MessageStreamID]; ok {
					r.WriteVideo(&RTMPVideo{msg.AVData})
					msg.AVData = RTMPData{}
					msg.AVData.ScalableMemoryAllocator = nc.ReadPool
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
