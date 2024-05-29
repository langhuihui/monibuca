package rtmp

import (
	"errors"
	"runtime"

	"m7s.live/m7s/v5"
)

type AVSender struct {
	*NetConnection
	ChunkHeader
	lastAbs uint32
}

func (av *AVSender) SendFrame(frame *RTMPData) (err error) {
	// seq := frame.Sequence
	payloadLen := frame.Size
	if payloadLen == 0 {
		err = errors.New("payload is empty")
		// av.Error("payload is empty", zap.Error(err))
		return err
	}
	if av.writeSeqNum > av.bandwidth {
		av.totalWrite += av.writeSeqNum
		av.writeSeqNum = 0
		av.SendMessage(RTMP_MSG_ACK, Uint32Message(av.totalWrite))
		av.SendStreamID(RTMP_USER_PING_REQUEST, 0)
	}
	av.MessageLength = uint32(payloadLen)
	for !av.writing.CompareAndSwap(false, true) {
		runtime.Gosched()
	}
	defer av.writing.Store(false)
	// 第一次是发送关键帧,需要完整的消息头(Chunk Basic Header(1) + Chunk Message Header(11) + Extended Timestamp(4)(可能会要包括))
	// 后面开始,就是直接发送音视频数据,那么直接发送,不需要完整的块(Chunk Basic Header(1) + Chunk Message Header(7))
	// 当Chunk Type为0时(即Chunk12),
	if av.lastAbs == 0 {
		av.SetTimestamp(frame.Timestamp)
		err = av.sendChunk(frame.Memory.Buffers, &av.ChunkHeader, RTMP_CHUNK_HEAD_12)
	} else {
		av.SetTimestamp(frame.Timestamp - av.lastAbs)
		err = av.sendChunk(frame.Memory.Buffers, &av.ChunkHeader, RTMP_CHUNK_HEAD_8)
	}
	av.lastAbs = frame.Timestamp
	// //数据被覆盖导致序号变了
	// if seq != frame.Sequence {
	// 	return errors.New("sequence is not equal")
	// }
	return err
}

//	func (rtmp *RTMPSender) OnEvent(event any) {
//		switch v := event.(type) {
//		case SEwaitPublish:
//			rtmp.Response(1, NetStream_Play_UnpublishNotify, Response_OnStatus)
//		case SEpublish:
//			rtmp.Response(1, NetStream_Play_PublishNotify, Response_OnStatus)
//		case ISubscriber:
//
//		case AudioDeConf:
//			rtmp.audio.sendSequenceHead(v)
//		case VideoDeConf:
//			rtmp.video.sendSequenceHead(v)
//		case AudioFrame:
//			if err := rtmp.audio.sendFrame(v.AVFrame, v.AbsTime); err != nil {
//				rtmp.Stop(zap.Error(err))
//			}
//		case VideoFrame:
//			if err := rtmp.video.sendFrame(v.AVFrame, v.AbsTime); err != nil {
//				rtmp.Stop(zap.Error(err))
//			}
//		default:
//			rtmp.Subscriber.OnEvent(event)
//		}
//	}

type RTMPReceiver struct {
	*m7s.Publisher
	NetStream
}
