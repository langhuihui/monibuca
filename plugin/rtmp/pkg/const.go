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
	*util.RecyclableBuffers
}

func (avcc *RTMPData) GetSize() int {
	return avcc.Length
}

func (avcc *RTMPData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Timestamp":%d,"Size":%d,"Data":"%s"}`, avcc.Timestamp, avcc.Length, avcc.String())), nil
}

func (avcc *RTMPData) String() string {
	return fmt.Sprintf("% 02X", avcc.Buffers.Buffers[0][:5])
}

func (avcc *RTMPData) GetTimestamp() time.Duration {
	return time.Duration(avcc.Timestamp) * time.Millisecond
}
func (avcc *RTMPData) IsIDR() bool {
	return false
}

func (avcc *RTMPData) WrapAudio() *RTMPAudio {
	return &RTMPAudio{RTMPData: *avcc}
}

func (avcc *RTMPData) WrapVideo() *RTMPVideo {
	return &RTMPVideo{RTMPData: *avcc}
}