package pkg

import (
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

const (
	PacketTypeSequenceStart = iota
	PacketTypeCodedFrames
	PacketTypeSequenceEnd
	PacketTypeCodedFramesX
	PacketTypeMetadata
	PacketTypeMPEG2TSSequenceStart
)

var FourCC_H265 = [4]byte{'H', '2', '6', '5'}
var FourCC_AV1 = [4]byte{'a', 'v', '0', '1'}

type RTMPData struct {
	Timestamp uint32
	util.Buffers
	util.RecyclebleMemory
}

func (avcc *RTMPData) GetTimestamp() time.Duration {
	return time.Duration(avcc.Timestamp) * time.Millisecond
}
func (avcc *RTMPData) IsIDR() bool {
	return false
}
