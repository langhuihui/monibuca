package rtmp

import (
	"fmt"

	"m7s.live/v5/pkg"
)

const (
	PacketTypeSequenceStart byte = iota
	PacketTypeCodedFrames
	PacketTypeSequenceEnd
	PacketTypeCodedFramesX
	PacketTypeMetadata
	PacketTypeMPEG2TSSequenceStart
)

type RTMPData struct {
	pkg.Sample
}

func (avcc *RTMPData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Timestamp":%d,"Size":%d,"Data":"%s"}`, avcc.Timestamp, avcc.Size, avcc.String())), nil
}

func (avcc *RTMPData) String() string {
	reader := avcc.NewReader()
	var bytes10 [10]byte
	reader.Read(bytes10[:])
	return fmt.Sprintf("%d % 02X", avcc.Timestamp, bytes10[:])
}
