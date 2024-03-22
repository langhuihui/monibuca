package pkg

import "m7s.live/m7s/v5"

// type AVSender struct {
// 	*RTMPSender
// 	ChunkHeader
// 	firstSent bool
// }

// func (av *AVSender) sendSequenceHead(seqHead []byte) {
// 	av.SetTimestamp(0)
// 	av.MessageLength = uint32(len(seqHead))
// 	for !av.writing.CompareAndSwap(false, true) {
// 		runtime.Gosched()
// 	}
// 	defer av.writing.Store(false)
// 	if av.firstSent {
// 		av.WriteTo(RTMP_CHUNK_HEAD_8, &av.chunkHeader)
// 	} else {
// 		av.WriteTo(RTMP_CHUNK_HEAD_12, &av.chunkHeader)
// 	}
// 	av.sendChunk(seqHead)
// }

// func (av *AVSender) sendFrame(frame *common.AVFrame, absTime uint32) (err error) {
// 	seq := frame.Sequence
// 	payloadLen := frame.AVCC.ByteLength
// 	if payloadLen == 0 {
// 		err := errors.New("payload is empty")
// 		av.Error("payload is empty", zap.Error(err))
// 		return err
// 	}
// 	if av.writeSeqNum > av.bandwidth {
// 		av.totalWrite += av.writeSeqNum
// 		av.writeSeqNum = 0
// 		av.SendMessage(RTMP_MSG_ACK, Uint32Message(av.totalWrite))
// 		av.SendStreamID(RTMP_USER_PING_REQUEST, 0)
// 	}
// 	av.MessageLength = uint32(payloadLen)
// 	for !av.writing.CompareAndSwap(false, true) {
// 		runtime.Gosched()
// 	}
// 	defer av.writing.Store(false)
// 	// 第一次是发送关键帧,需要完整的消息头(Chunk Basic Header(1) + Chunk Message Header(11) + Extended Timestamp(4)(可能会要包括))
// 	// 后面开始,就是直接发送音视频数据,那么直接发送,不需要完整的块(Chunk Basic Header(1) + Chunk Message Header(7))
// 	// 当Chunk Type为0时(即Chunk12),
// 	if !av.firstSent {
// 		av.firstSent = true
// 		av.SetTimestamp(absTime)
// 		av.WriteTo(RTMP_CHUNK_HEAD_12, &av.chunkHeader)
// 	} else {
// 		av.SetTimestamp(frame.DeltaTime)
// 		av.WriteTo(RTMP_CHUNK_HEAD_8, &av.chunkHeader)
// 	}
// 	//数据被覆盖导致序号变了
// 	if seq != frame.Sequence {
// 		return errors.New("sequence is not equal")
// 	}
// 	r := frame.AVCC.NewReader()
// 	chunk := net.Buffers{av.chunkHeader}
// 	av.writeSeqNum += uint32(av.chunkHeader.Len() + r.WriteNTo(av.writeChunkSize, &chunk))
// 	for r.CanRead() {
// 		item := av.bytePool.Get(16)
// 		defer item.Recycle()
// 		av.WriteTo(RTMP_CHUNK_HEAD_1, &item.Value)
// 		// 如果在音视频数据太大,一次发送不完,那么这里进行分割(data + Chunk Basic Header(1))
// 		chunk = append(chunk, item.Value)
// 		av.writeSeqNum += uint32(item.Value.Len() + r.WriteNTo(av.writeChunkSize, &chunk))
// 	}
// 	_, err = chunk.WriteTo(av.Conn)
// 	return nil
// }

// type RTMPSender struct {
// 	Subscriber
// 	NetStream
// 	audio, video AVSender
// }

// func (rtmp *RTMPSender) OnEvent(event any) {
// 	switch v := event.(type) {
// 	case SEwaitPublish:
// 		rtmp.Response(1, NetStream_Play_UnpublishNotify, Response_OnStatus)
// 	case SEpublish:
// 		rtmp.Response(1, NetStream_Play_PublishNotify, Response_OnStatus)
// 	case ISubscriber:
// 		rtmp.audio.RTMPSender = rtmp
// 		rtmp.video.RTMPSender = rtmp
// 		rtmp.audio.ChunkStreamID = RTMP_CSID_AUDIO
// 		rtmp.video.ChunkStreamID = RTMP_CSID_VIDEO
// 		rtmp.audio.MessageTypeID = RTMP_MSG_AUDIO
// 		rtmp.video.MessageTypeID = RTMP_MSG_VIDEO
// 		rtmp.audio.MessageStreamID = rtmp.StreamID
// 		rtmp.video.MessageStreamID = rtmp.StreamID
// 	case AudioDeConf:
// 		rtmp.audio.sendSequenceHead(v)
// 	case VideoDeConf:
// 		rtmp.video.sendSequenceHead(v)
// 	case AudioFrame:
// 		if err := rtmp.audio.sendFrame(v.AVFrame, v.AbsTime); err != nil {
// 			rtmp.Stop(zap.Error(err))
// 		}
// 	case VideoFrame:
// 		if err := rtmp.video.sendFrame(v.AVFrame, v.AbsTime); err != nil {
// 			rtmp.Stop(zap.Error(err))
// 		}
// 	default:
// 		rtmp.Subscriber.OnEvent(event)
// 	}
// }

// func (r *RTMPSender) Response(tid uint64, code, level string) error {
// 	m := new(ResponsePlayMessage)
// 	m.CommandName = Response_OnStatus
// 	m.TransactionId = tid
// 	m.Infomation = map[string]any{
// 		"code":        code,
// 		"level":       level,
// 		"description": "",
// 	}
// 	m.StreamID = r.StreamID
// 	return r.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
// }

type RTMPReceiver struct {
	*m7s.Publisher
	NetStream
}

func (r *RTMPReceiver) Response(tid uint64, code, level string) error {
	m := new(ResponsePublishMessage)
	m.CommandName = Response_OnStatus
	m.TransactionId = tid
	m.Infomation = map[string]any{
		"code":        code,
		"level":       level,
		"description": "",
	}
	m.StreamID = r.StreamID
	return r.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
}

func (r *RTMPReceiver) ReceiveAudio(msg *Chunk) {
	// r.WriteAudio(nil)
}

func (r *RTMPReceiver) ReceiveVideo(msg *Chunk) {
	r.WriteVideo(&RTMPVideo{msg.AVData})
}
