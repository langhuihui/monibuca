package mpegps

import (
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/util"
)

type MpegpsPESFrame struct {
	StreamType byte // Stream type (e.g., video, audio)
	mpegts.MpegPESHeader
}

func (frame *MpegpsPESFrame) WritePESPacket(payload util.Memory, allocator *util.RecyclableMemory) (err error) {
	frame.DataAlignmentIndicator = 1

	pesReader := payload.NewReader()
	var outputMemory util.Buffer = allocator.NextN(PSPackHeaderSize)
	outputMemory.Reset()
	MuxPSHeader(&outputMemory)
	for pesReader.Length > 0 {
		currentPESPayload := min(pesReader.Length, MaxPESPayloadSize)
		var pesHeadItem util.Buffer
		pesHeadItem, err = frame.WritePESHeader(currentPESPayload)
		if err != nil {
			return
		}
		copy(allocator.NextN(pesHeadItem.Len()), pesHeadItem)
		// 申请输出缓冲
		outputMemory = allocator.NextN(currentPESPayload)
		pesReader.Read(outputMemory)
		frame.DataAlignmentIndicator = 0
	}

	return nil
}
