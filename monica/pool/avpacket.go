package pool

import (
	"sync"
)

var (
	AVPacketPool = &sync.Pool{
		New: func() interface{} {
			return new(AVPacket)
		},
	}
	SendPacketPool = &sync.Pool{
		New: func() interface{} {
			return new(SendPacket)
		},
	}
)

// Video or Audio
type AVPacket struct {
	Timestamp     uint32
	Type          byte //8 audio,9 video
	IsAACSequence bool
	IsADTS        bool
	// Video
	VideoFrameType byte //4bit
	IsAVCSequence  bool
	Payload        []byte
	RefCount       int //Payload的引用次数
}

func (av *AVPacket) IsKeyFrame() bool {
	return av.VideoFrameType == 1 || av.VideoFrameType == 4
}

func (av *AVPacket) Recycle() {
	if av.RefCount == 0 {
		return
	} else if av.RefCount == 1 {
		av.RefCount = 0
		RecycleSlice(av.Payload)
		AVPacketPool.Put(av)
	} else {
		av.RefCount--
	}
}
func NewAVPacket(avType byte) (p *AVPacket) {
	p = AVPacketPool.Get().(*AVPacket)
	p.Type = avType
	p.IsAVCSequence = false
	p.VideoFrameType = 0
	p.Timestamp = 0
	p.IsAACSequence = false
	p.IsADTS = false
	return
}

type SendPacket struct {
	Timestamp uint32
	Packet    *AVPacket
}

func (packet *SendPacket) Recycle() {
	packet.Packet.Recycle()
	SendPacketPool.Put(packet)
}
func NewSendPacket(p *AVPacket, timestamp uint32) (result *SendPacket) {
	result = SendPacketPool.Get().(*SendPacket)
	result.Packet = p
	result.Timestamp = timestamp
	return
}
