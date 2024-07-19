package box

import (
	"encoding/binary"
	"io"

	"github.com/yapingcat/gomedia/go-codec"
)

// class ChannelMappingTable (unsigned int(8) OutputChannelCount){
//     unsigned int(8) StreamCount;
//     unsigned int(8) CoupledCount;
//     unsigned int(8 * OutputChannelCount) ChannelMapping;
// }
// aligned(8) class OpusSpecificBox extends Box('dOps'){
//     unsigned int(8) Version;
//     unsigned int(8) OutputChannelCount;
//     unsigned int(16) PreSkip;
//     unsigned int(32) InputSampleRate;
//     signed int(16) OutputGain;
//     unsigned int(8) ChannelMappingFamily;
//     if (ChannelMappingFamily != 0) {
//         ChannelMappingTable(OutputChannelCount);
//     }
// }

type ChannelMappingTable struct {
	StreamCount    uint8
	CoupledCount   uint8
	ChannelMapping []byte
}

type OpusSpecificBox struct {
	Box                *BasicBox
	Version            uint8
	OutputChannelCount uint8
	PreSkip            uint16
	InputSampleRate    uint32
	OutputGain         int16
	ChanMapTable       *ChannelMappingTable
}

func NewdOpsBox() *OpusSpecificBox {
	return &OpusSpecificBox{
		Box: NewBasicBox([4]byte{'d', 'O', 'p', 's'}),
	}
}

func (dops *OpusSpecificBox) Size() uint64 {
	return uint64(8 + 10 + 2 + dops.OutputChannelCount)
}

func (dops *OpusSpecificBox) Encode() (int, []byte) {
	dops.Box.Size = dops.Size()
	offset, buf := dops.Box.Encode()
	buf[offset] = dops.Version
	offset++
	buf[offset] = dops.OutputChannelCount
	offset++
	binary.LittleEndian.PutUint16(buf[offset:], dops.PreSkip)
	offset += 2
	binary.BigEndian.PutUint32(buf[offset:], dops.InputSampleRate)
	offset += 4
	binary.LittleEndian.PutUint16(buf[offset:], uint16(dops.OutputGain))
	offset += 2
	if dops.ChanMapTable != nil {
		buf[offset] = dops.ChanMapTable.StreamCount
		offset++
		buf[offset] = dops.ChanMapTable.CoupledCount
		offset++
		copy(buf[offset:], dops.ChanMapTable.ChannelMapping)
		offset += len(dops.ChanMapTable.ChannelMapping)
	}
	return offset, buf
}

func (dops *OpusSpecificBox) Decode(r io.Reader, size uint32) (offset int, err error) {

	dopsBuf := make([]byte, size-BasicBoxLen)
	ChannelMappingFamily := 0
	if size-BasicBoxLen-10 > 0 {
		ChannelMappingFamily = int(size - BasicBoxLen - 10)
	}

	if _, err = io.ReadFull(r, dopsBuf); err != nil {
		return
	}

	dops.Version = dopsBuf[0]
	dops.OutputChannelCount = dopsBuf[1]
	dops.PreSkip = binary.BigEndian.Uint16(dopsBuf[2:])
	dops.InputSampleRate = binary.BigEndian.Uint32(dopsBuf[4:])
	dops.OutputGain = int16(binary.BigEndian.Uint16(dopsBuf[8:]))
	dops.ChanMapTable = nil
	if ChannelMappingFamily > 0 {
		dops.ChanMapTable = &ChannelMappingTable{}
		dops.ChanMapTable.StreamCount = dopsBuf[10]
		dops.ChanMapTable.CoupledCount = dopsBuf[11]
		dops.ChanMapTable.ChannelMapping = make([]byte, ChannelMappingFamily-2)
		copy(dops.ChanMapTable.ChannelMapping, dopsBuf[12:])
	}

	return int(size - BasicBoxLen), nil
}

func makeOpusSpecificBox(extraData []byte) []byte {
	ctx := &codec.OpusContext{}
	ctx.ParseExtranData(extraData)
	dops := NewdOpsBox()
	dops.Version = 0
	dops.OutputChannelCount = uint8(ctx.ChannelCount)
	dops.PreSkip = uint16(ctx.Preskip)
	dops.InputSampleRate = uint32(ctx.SampleRate)
	dops.OutputGain = int16(ctx.OutputGain)
	if ctx.MapType > 0 {
		dops.ChanMapTable = &ChannelMappingTable{
			StreamCount:    uint8(ctx.StreamCount),
			CoupledCount:   uint8(ctx.StereoStreamCount),
			ChannelMapping: make([]byte, len(ctx.Channel)),
		}
		copy(dops.ChanMapTable.ChannelMapping, ctx.Channel)
	}
	_, dopsbox := dops.Encode()
	return dopsbox
}
