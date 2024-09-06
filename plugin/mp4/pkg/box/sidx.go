package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SegmentIndexBox extends FullBox(‘sidx’, version, 0) {
//    unsigned int(32) reference_ID;
//    unsigned int(32) timescale;
//    if (version==0) {
//          unsigned int(32) earliest_presentation_time;
//          unsigned int(32) first_offset;
//    }
//    else {
//          unsigned int(64) earliest_presentation_time;
//          unsigned int(64) first_offset;
//    }
//    unsigned int(16) reserved = 0;
//    unsigned int(16) reference_count;
//    for(i=1; i <= reference_count; i++)
//    {
//       bit (1)           reference_type;
//       unsigned int(31)  referenced_size;
//       unsigned int(32)  subsegment_duration;
//       bit(1)            starts_with_SAP;
//       unsigned int(3)   SAP_type;
//       unsigned int(28)  SAP_delta_time;
//    }
// }

type sidxEntry struct {
	ReferenceType      uint8
	ReferencedSize     uint32
	SubsegmentDuration uint32
	StartsWithSAP      uint8
	SAPType            uint8
	SAPDeltaTime       uint32
}

type SegmentIndexBox struct {
	Box                      *FullBox
	ReferenceID              uint32
	TimeScale                uint32
	EarliestPresentationTime uint64
	FirstOffset              uint64
	ReferenceCount           uint16
	Entrys                   []sidxEntry
}

func NewSegmentIndexBox() *SegmentIndexBox {
	return &SegmentIndexBox{
		Box: NewFullBox([4]byte{'s', 'i', 'd', 'x'}, 1),
	}
}

func (sidx *SegmentIndexBox) Size() uint64 {
	return sidx.Box.Size() + 28 + uint64(len(sidx.Entrys)*12)
}

func (sidx *SegmentIndexBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = sidx.Box.Decode(r); err != nil {
		return
	}
	buf := make([]byte, sidx.Box.Box.Size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	n := 0
	sidx.ReferenceID = binary.BigEndian.Uint32(buf[n:])
	n += 4
	sidx.TimeScale = binary.BigEndian.Uint32(buf[n:])
	n += 4
	if sidx.Box.Version == 0 {
		sidx.EarliestPresentationTime = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		sidx.FirstOffset = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	} else {
		sidx.EarliestPresentationTime = binary.BigEndian.Uint64(buf[n:])
		n += 8
		sidx.FirstOffset = binary.BigEndian.Uint64(buf[n:])
		n += 8
	}
	n += 2
	sidx.ReferenceCount = binary.BigEndian.Uint16(buf[n:])
	n += 2
	sidx.Entrys = make([]sidxEntry, sidx.ReferenceCount)
	for i := 0; i < int(sidx.ReferenceCount); i++ {
		sidx.Entrys[i].ReferenceType = buf[n] >> 7
		buf[n] = buf[n] & 0x7F
		sidx.Entrys[i].ReferencedSize = binary.BigEndian.Uint32(buf[n:])
		n += 4
		sidx.Entrys[i].SubsegmentDuration = binary.BigEndian.Uint32(buf[n:])
		n += 4
		sidx.Entrys[i].StartsWithSAP = buf[n] >> 7
		sidx.Entrys[i].SAPType = buf[n] >> 4 & 0x07
		buf[n] = buf[n] & 0x0F
		sidx.Entrys[i].SAPDeltaTime = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	offset += 4
	return
}

func (sidx *SegmentIndexBox) Encode() (int, []byte) {
	sidx.Box.Box.Size = sidx.Size()
	offset, boxdata := sidx.Box.Encode()
	binary.BigEndian.PutUint32(boxdata[offset:], sidx.ReferenceID)
	offset += 4
	binary.BigEndian.PutUint32(boxdata[offset:], sidx.TimeScale)
	offset += 4
	if sidx.Box.Version == 0 {
		binary.BigEndian.PutUint32(boxdata[offset:], uint32(sidx.EarliestPresentationTime))
		offset += 4
		binary.BigEndian.PutUint32(boxdata[offset:], uint32(sidx.FirstOffset))
		offset += 4
	} else {
		binary.BigEndian.PutUint64(boxdata[offset:], sidx.EarliestPresentationTime)
		offset += 8
		binary.BigEndian.PutUint64(boxdata[offset:], sidx.FirstOffset)
		offset += 8
	}
	offset += 2
	binary.BigEndian.PutUint16(boxdata[offset:], sidx.ReferenceCount)
	offset += 2
	for i := 0; i < int(sidx.ReferenceCount); i++ {
		binary.BigEndian.PutUint32(boxdata[offset:], uint32(sidx.Entrys[i].ReferencedSize))
		boxdata[offset] = boxdata[offset]&0x7F | sidx.Entrys[i].ReferenceType<<7
		offset += 4
		binary.BigEndian.PutUint32(boxdata[offset:], sidx.Entrys[i].SubsegmentDuration)
		offset += 4
		binary.BigEndian.PutUint32(boxdata[offset:], sidx.Entrys[i].SAPDeltaTime)
		boxdata[offset] = (boxdata[offset] & 0xF0) + sidx.Entrys[i].StartsWithSAP<<7 | (sidx.Entrys[i].SAPType&0x07)<<4
		offset += 4
	}
	return offset, boxdata
}

func makeSidxBox(track *mp4track, totalSidxSize uint32, refsize uint32) []byte {
	sidx := NewSegmentIndexBox()
	sidx.ReferenceID = track.trackId
	sidx.TimeScale = track.timescale
	sidx.EarliestPresentationTime = track.startPts
	sidx.ReferenceCount = 1
	sidx.FirstOffset = 52 + uint64(totalSidxSize)
	entry := sidxEntry{
		ReferenceType:      0,
		ReferencedSize:     refsize,
		SubsegmentDuration: 0,
		StartsWithSAP:      1,
		SAPType:            0,
		SAPDeltaTime:       0,
	}

	if len(track.samplelist) > 0 {
		entry.SubsegmentDuration = uint32(track.samplelist[len(track.samplelist)-1].dts) - uint32(track.startDts)
	}
	sidx.Entrys = append(sidx.Entrys, entry)
	sidx.Box.Box.Size = sidx.Size()
	_, boxData := sidx.Encode()
	return boxData
}
