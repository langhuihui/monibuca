package rtmp

import (
	"fmt"
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

type RTMPData struct {
	Timestamp uint32
	util.Buffers
	util.RecyclableMemory
}

func (avcc *RTMPData) GetSize() int {
	return avcc.Length
}

func (avcc *RTMPData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Timestamp":%d,"Size":%d,"Data":"%s"}`, avcc.Timestamp, avcc.Length, avcc.Print())), nil
}

func (avcc *RTMPData) Print() string {
	return fmt.Sprintf("% 02X", avcc.Buffers.Buffers[0][:5])
}

func (avcc *RTMPData) GetTimestamp() time.Duration {
	return time.Duration(avcc.Timestamp) * time.Millisecond
}
func (avcc *RTMPData) IsIDR() bool {
	return false
}
