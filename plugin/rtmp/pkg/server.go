package pkg

type NetStream struct {
	*NetConnection
	StreamID uint32
}

func (ns *NetStream) Begin() {
	ns.SendStreamID(RTMP_USER_STREAM_BEGIN, ns.StreamID)
}

// type RTMPSubscriber struct {
// 	RTMPSender
// }

// func (s *RTMPSubscriber) OnEvent(event any) {
// 	switch event.(type) {
// 	case engine.SEclose:
// 		s.Response(0, NetStream_Play_Stop, Level_Status)
// 	}
// 	s.RTMPSender.OnEvent(event)
// }
// func (config *RTMPConfig) ServeTCP(conn net.Conn) {
// 	defer conn.Close()
// 	logger := RTMPPlugin.Logger.With(zap.String("remote", conn.RemoteAddr().String()))
// 	senders := make(map[uint32]*RTMPSubscriber)
// 	receivers := make(map[uint32]*RTMPReceiver)
// 	var err error
// 	logger.Info("conn")
// 	defer func() {
// 		ze := zap.Error(err)
// 		logger.Info("conn close", ze)
// 		for _, sender := range senders {
// 			sender.Stop(ze)
// 		}
// 		for _, receiver := range receivers {
// 			receiver.Stop(ze)
// 		}
// 	}()
// 	nc := NewNetConnection(conn)
// 	ctx, cancel := context.WithCancel(engine.Engine)
// 	defer cancel()
// 	/* Handshake */
// 	if err = nc.Handshake(); err != nil {
// 		logger.Error("handshake", zap.Error(err))
// 		return
// 	}
// 	var msg *Chunk
// 	var gstreamid uint32
// 	for {
// 		if msg, err = nc.RecvMessage(); err == nil {
// 			if msg.MessageLength <= 0 {
// 				continue
// 			}
// 			switch msg.MessageTypeID {
// 			case RTMP_MSG_AMF0_COMMAND:
// 				if msg.MsgData == nil {
// 					break
// 				}
// 				cmd := msg.MsgData.(Commander).GetCommand()
// 				logger.Debug("recv cmd", zap.String("commandName", cmd.CommandName), zap.Uint32("streamID", msg.MessageStreamID))
// 				switch cmd := msg.MsgData.(type) {
// 				case *CallMessage: //connect
// 					app := cmd.Object["app"]                       // 客户端要连接到的服务应用名
// 					objectEncoding := cmd.Object["objectEncoding"] // AMF编码方法
// 					switch v := objectEncoding.(type) {
// 					case float64:
// 						nc.objectEncoding = v
// 					default:
// 						nc.objectEncoding = 0
// 					}
// 					nc.appName = app.(string)
// 					logger.Info("connect", zap.String("appName", nc.appName), zap.Float64("objectEncoding", nc.objectEncoding))
// 					err = nc.SendMessage(RTMP_MSG_ACK_SIZE, Uint32Message(512<<10))
// 					nc.writeChunkSize = config.ChunkSize
// 					err = nc.SendMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(config.ChunkSize))
// 					err = nc.SendMessage(RTMP_MSG_BANDWIDTH, &SetPeerBandwidthMessage{
// 						AcknowledgementWindowsize: uint32(512 << 10),
// 						LimitType:                 byte(2),
// 					})
// 					err = nc.SendStreamID(RTMP_USER_STREAM_BEGIN, 0)
// 					m := new(ResponseConnectMessage)
// 					m.CommandName = Response_Result
// 					m.TransactionId = 1
// 					m.Properties = map[string]any{
// 						"fmsVer":       "monibuca/" + engine.Engine.Version,
// 						"capabilities": 31,
// 						"mode":         1,
// 						"Author":       "dexter",
// 					}
// 					m.Infomation = map[string]any{
// 						"level":          Level_Status,
// 						"code":           NetConnection_Connect_Success,
// 						"objectEncoding": nc.objectEncoding,
// 					}
// 					err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
// 				case *CommandMessage: // "createStream"
// 					gstreamid++
// 					logger.Info("createStream:", zap.Uint32("streamId", gstreamid))
// 					nc.ResponseCreateStream(cmd.TransactionId, gstreamid)
// 				case *CURDStreamMessage:
// 					if stream, ok := receivers[cmd.StreamId]; ok {
// 						stream.Stop()
// 						delete(senders, cmd.StreamId)
// 					}
// 				case *ReleaseStreamMessage:
// 					// m := &CommandMessage{
// 					// 	CommandName:   "releaseStream_error",
// 					// 	TransactionId: cmd.TransactionId,
// 					// }
// 					// s := engine.Streams.Get(nc.appName + "/" + cmd.StreamName)
// 					// if s != nil && s.Publisher != nil {
// 					// 	if p, ok := s.Publisher.(*RTMPReceiver); ok {
// 					// 		// m.CommandName = "releaseStream_result"
// 					// 		p.Stop()
// 					// 		delete(receivers, p.StreamID)
// 					// 	}
// 					// }
// 					// err = nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
// 				case *PublishMessage:
// 					receiver := &RTMPReceiver{
// 						NetStream: NetStream{
// 							NetConnection: nc,
// 							StreamID:      cmd.StreamId,
// 						},
// 					}
// 					receiver.SetParentCtx(ctx)
// 					if !config.KeepAlive {
// 						receiver.SetIO(conn)
// 					}
// 					if RTMPPlugin.Publish(nc.appName+"/"+cmd.PublishingName, receiver) == nil {
// 						receivers[cmd.StreamId] = receiver
// 						receiver.Begin()
// 						err = receiver.Response(cmd.TransactionId, NetStream_Publish_Start, Level_Status)
// 					} else {
// 						delete(receivers, cmd.StreamId)
// 						err = receiver.Response(cmd.TransactionId, NetStream_Publish_BadName, Level_Error)
// 					}
// 				case *PlayMessage:
// 					streamPath := nc.appName + "/" + cmd.StreamName
// 					sender := &RTMPSubscriber{}
// 					sender.NetStream = NetStream{
// 						nc,
// 						cmd.StreamId,
// 					}
// 					sender.SetParentCtx(ctx)
// 					if !config.KeepAlive {
// 						sender.SetIO(conn)
// 					}
// 					sender.ID = fmt.Sprintf("%s|%d", conn.RemoteAddr().String(), sender.StreamID)
// 					if RTMPPlugin.Subscribe(streamPath, sender) != nil {
// 						sender.Response(cmd.TransactionId, NetStream_Play_Failed, Level_Error)
// 					} else {
// 						senders[sender.StreamID] = sender
// 						sender.Begin()
// 						sender.Response(cmd.TransactionId, NetStream_Play_Reset, Level_Status)
// 						sender.Response(cmd.TransactionId, NetStream_Play_Start, Level_Status)
// 						go sender.PlayRaw()
// 					}
// 				}
// 			case RTMP_MSG_AUDIO:
// 				if r, ok := receivers[msg.MessageStreamID]; ok {
// 					r.ReceiveAudio(msg)
// 				} else {
// 					logger.Warn("ReceiveAudio", zap.Uint32("MessageStreamID", msg.MessageStreamID))
// 				}
// 			case RTMP_MSG_VIDEO:
// 				if r, ok := receivers[msg.MessageStreamID]; ok {
// 					r.ReceiveVideo(msg)
// 				} else {
// 					logger.Warn("ReceiveVideo", zap.Uint32("MessageStreamID", msg.MessageStreamID))
// 				}
// 			}
// 		} else if err == io.EOF || err == io.ErrUnexpectedEOF {
// 			logger.Info("rtmp client closed")
// 			return
// 		} else {
// 			logger.Warn("ReadMessage", zap.Error(err))
// 			return
// 		}
// 	}
// }
