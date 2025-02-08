package box

import (
	"encoding/binary"
	"io"
	"net"

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
	BaseBox
	Version            uint8
	OutputChannelCount uint8
	PreSkip            uint16
	InputSampleRate    uint32
	OutputGain         int16
	ChanMapTable       *ChannelMappingTable
}

func CreateOpusSpecificBox(extraData []byte) *OpusSpecificBox {
	ctx := &codec.OpusContext{}
	ctx.ParseExtranData(extraData)
	dops := &OpusSpecificBox{
		BaseBox: BaseBox{
			typ:  TypeDOPS,
			size: 0,
		},
	}
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

	// Calculate final size
	dops.size = uint32(BasicBoxLen + 10) // Base size
	if dops.ChanMapTable != nil {
		dops.size += uint32(2 + len(dops.ChanMapTable.ChannelMapping))
	}
	return dops
}

func (box *OpusSpecificBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [12]byte                   // Buffer for fixed-size fields
	buffers := make(net.Buffers, 0, 4) // Estimate initial capacity

	// Write fixed fields
	tmp[0] = box.Version
	tmp[1] = box.OutputChannelCount
	binary.BigEndian.PutUint16(tmp[2:], box.PreSkip)
	binary.BigEndian.PutUint32(tmp[4:], box.InputSampleRate)
	binary.BigEndian.PutUint16(tmp[8:], uint16(box.OutputGain))

	if box.ChanMapTable != nil {
		tmp[10] = box.ChanMapTable.StreamCount
		tmp[11] = box.ChanMapTable.CoupledCount
		buffers = append(buffers, tmp[:12])
		buffers = append(buffers, box.ChanMapTable.ChannelMapping)
	} else {
		buffers = append(buffers, tmp[:10])
	}

	return buffers.WriteTo(w)
}

func (box *OpusSpecificBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 10 {
		return nil, io.ErrShortBuffer
	}

	box.Version = buf[0]
	box.OutputChannelCount = buf[1]
	box.PreSkip = binary.BigEndian.Uint16(buf[2:])
	box.InputSampleRate = binary.BigEndian.Uint32(buf[4:])
	box.OutputGain = int16(binary.BigEndian.Uint16(buf[8:]))

	// Check if we have channel mapping data
	if len(buf) > 10 {
		if len(buf) < 12 {
			return nil, io.ErrShortBuffer
		}
		box.ChanMapTable = &ChannelMappingTable{
			StreamCount:  buf[10],
			CoupledCount: buf[11],
		}
		if len(buf) > 12 {
			box.ChanMapTable.ChannelMapping = make([]byte, len(buf)-12)
			copy(box.ChanMapTable.ChannelMapping, buf[12:])
		}
	}

	return box, nil
}

func init() {
	RegisterBox[*OpusSpecificBox](TypeDOPS)
}
