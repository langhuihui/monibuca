package rtmp

import (
	"errors"
	"m7s.live/v5/pkg"
	"runtime"

	"m7s.live/v5"
)

type Sender struct {
	*NetConnection
	ChunkHeader
	errContinue bool
	lastAbs     uint32
}

func (av *Sender) HandleAudio(frame *RTMPAudio) (err error) {
	return av.SendFrame(&frame.RTMPData)
}

func (av *Sender) HandleVideo(frame *RTMPVideo) (err error) {
	return av.SendFrame(&frame.RTMPData)
}

func (av *Sender) SendFrame(frame *RTMPData) (err error) {
	// seq := frame.Sequence
	payloadLen := frame.Size
	if av.errContinue {
		defer func() {
			if err != nil {
				err = pkg.ErrInterrupt
			}
		}()
	}
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
	return
}

type Receiver struct {
	*m7s.Publisher
	NetStream
}
