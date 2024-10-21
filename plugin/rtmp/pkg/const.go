package rtmp

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"m7s.live/v5/pkg/util"
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
	Timestamp uint32
	util.RecyclableMemory
}

func (avcc *RTMPData) Dump(t byte, w io.Writer) {
	m := avcc.GetAllocator().Borrow(9 + avcc.Size)
	m[0] = t
	binary.BigEndian.PutUint32(m[1:], uint32(4+avcc.Size))
	binary.BigEndian.PutUint32(m[5:], avcc.Timestamp)
	avcc.CopyTo(m[9:])
	w.Write(m)
}

func (avcc *RTMPData) GetSize() int {
	return avcc.Size
}

func (avcc *RTMPData) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Timestamp":%d,"Size":%d,"Data":"%s"}`, avcc.Timestamp, avcc.Size, avcc.String())), nil
}

func (avcc *RTMPData) String() string {
	reader := avcc.NewReader()
	var bytes10 [10]byte
	reader.ReadBytesTo(bytes10[:])
	return fmt.Sprintf("%d % 02X", avcc.Timestamp, bytes10[:])
}

func (avcc *RTMPData) GetTimestamp() time.Duration {
	return time.Duration(avcc.Timestamp) * time.Millisecond
}

func (avcc *RTMPData) GetCTS() time.Duration {
	return 0
}

func (avcc *RTMPData) WrapAudio() *RTMPAudio {
	return &RTMPAudio{RTMPData: *avcc}
}

func (avcc *RTMPData) WrapVideo() *RTMPVideo {
	return &RTMPVideo{RTMPData: *avcc}
}
