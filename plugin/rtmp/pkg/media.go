package rtmp

import (
	"errors"
	"runtime"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
)

type AVSender struct {
	*RTMPSender
	ChunkHeader
	lastAbs uint32
}

func (av *AVSender) sendFrame(frame *RTMPData) (err error) {
	// seq := frame.Sequence
	payloadLen := frame.Length
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
		av.WriteTo(RTMP_CHUNK_HEAD_12, &av.chunkHeader)
	} else {
		av.SetTimestamp(frame.Timestamp - av.lastAbs)
		av.WriteTo(RTMP_CHUNK_HEAD_8, &av.chunkHeader)
	}
	av.lastAbs = frame.Timestamp
	// //数据被覆盖导致序号变了
	// if seq != frame.Sequence {
	// 	return errors.New("sequence is not equal")
	// }
	r := frame.Buffers
	chunkHeader := av.chunkHeader
	av.chunk = append(av.chunk, chunkHeader)
	// var buffer util.Buffer = r.ToBytes()
	av.writeSeqNum += uint32(chunkHeader.Len() + r.WriteNTo(av.WriteChunkSize, &av.chunk))
	if r.Length > 0 {
		defer av.mem.Recycle()
		for r.Length > 0 {
			chunkHeader = av.mem.Malloc(5)
			av.WriteTo(RTMP_CHUNK_HEAD_1, &chunkHeader)
			// 如果在音视频数据太大,一次发送不完,那么这里进行分割(data + Chunk Basic Header(1))
			av.chunk = append(av.chunk, chunkHeader)
			av.writeSeqNum += uint32(chunkHeader.Len() + r.WriteNTo(av.WriteChunkSize, &av.chunk))
		}
	}
	_, err = av.chunk.WriteTo(av.Conn)
	return err
}

type RTMPSender struct {
	*m7s.Subscriber
	NetStream
	audio, video AVSender
	mem          util.RecyclableMemory
}

func (r *RTMPSender) Init() {
	r.audio.RTMPSender = r
	r.video.RTMPSender = r
	r.audio.ChunkStreamID = RTMP_CSID_AUDIO
	r.video.ChunkStreamID = RTMP_CSID_VIDEO
	r.audio.MessageTypeID = RTMP_MSG_AUDIO
	r.video.MessageTypeID = RTMP_MSG_VIDEO
	r.audio.MessageStreamID = r.StreamID
	r.video.MessageStreamID = r.StreamID
	r.mem.MemoryAllocator = r.ByteChunkPool
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

func (r *RTMPSender) SendAudio(audio *RTMPAudio) error {
	return r.audio.sendFrame(&audio.RTMPData)
}

func (r *RTMPSender) SendVideo(video *RTMPVideo) error {
	return r.video.sendFrame(&video.RTMPData)
}

type RTMPReceiver struct {
	*m7s.Publisher
	NetStream
}
