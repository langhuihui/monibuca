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

type SidxEntry struct {
	ReferenceType      uint8
	ReferencedSize     uint32
	SubsegmentDuration uint32
	StartsWithSAP      uint8
	SAPType            uint8
	SAPDeltaTime       uint32
}

type SegmentIndexBox struct {
	FullBox
	ReferenceID              uint32
	TimeScale                uint32
	EarliestPresentationTime uint64
	FirstOffset              uint64
	ReferenceCount           uint16
	Entries                  []SidxEntry
}

func CreateSegmentIndexBox(referenceID uint32, timeScale uint32, earliestPresentationTime uint64, firstOffset uint64, entries []SidxEntry) *SegmentIndexBox {
	version := uint8(0)
	if earliestPresentationTime > 0xFFFFFFFF || firstOffset > 0xFFFFFFFF {
		version = 1
	}

	size := uint32(FullBoxLen + 12) // base size + referenceID(4) + timeScale(4) + reserved(2) + referenceCount(2)
	if version == 1 {
		size += 16 // earliestPresentationTime(8) + firstOffset(8)
	} else {
		size += 8 // earliestPresentationTime(4) + firstOffset(4)
	}
	size += uint32(len(entries) * 12) // each entry is 12 bytes

	return &SegmentIndexBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSIDX,
				size: size,
			},
			Version: version,
			Flags:   [3]byte{0, 0, 0},
		},
		ReferenceID:              referenceID,
		TimeScale:                timeScale,
		EarliestPresentationTime: earliestPresentationTime,
		FirstOffset:              firstOffset,
		ReferenceCount:           uint16(len(entries)),
		Entries:                  entries,
	}
}

func (box *SegmentIndexBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	binary.BigEndian.PutUint32(tmp[:4], box.ReferenceID)
	if _, err = w.Write(tmp[:4]); err != nil {
		return
	}
	n = 4

	binary.BigEndian.PutUint32(tmp[:4], box.TimeScale)
	if _, err = w.Write(tmp[:4]); err != nil {
		return
	}
	n += 4

	if box.Version == 0 {
		binary.BigEndian.PutUint32(tmp[:4], uint32(box.EarliestPresentationTime))
		if _, err = w.Write(tmp[:4]); err != nil {
			return
		}
		binary.BigEndian.PutUint32(tmp[:4], uint32(box.FirstOffset))
		if _, err = w.Write(tmp[:4]); err != nil {
			return
		}
		n += 8
	} else {
		binary.BigEndian.PutUint64(tmp[:], box.EarliestPresentationTime)
		if _, err = w.Write(tmp[:]); err != nil {
			return
		}
		binary.BigEndian.PutUint64(tmp[:], box.FirstOffset)
		if _, err = w.Write(tmp[:]); err != nil {
			return
		}
		n += 16
	}

	binary.BigEndian.PutUint16(tmp[:2], 0) // reserved
	if _, err = w.Write(tmp[:2]); err != nil {
		return
	}
	n += 2

	binary.BigEndian.PutUint16(tmp[:2], box.ReferenceCount)
	if _, err = w.Write(tmp[:2]); err != nil {
		return
	}
	n += 2

	for _, entry := range box.Entries {
		var entryBuf [12]byte
		entryBuf[0] = entry.ReferenceType << 7
		binary.BigEndian.PutUint32(entryBuf[:4], entry.ReferencedSize)
		entryBuf[0] &= 0x7F
		binary.BigEndian.PutUint32(entryBuf[4:], entry.SubsegmentDuration)
		entryBuf[8] = entry.StartsWithSAP << 7
		entryBuf[8] |= (entry.SAPType & 0x07) << 4
		binary.BigEndian.PutUint32(entryBuf[8:], entry.SAPDeltaTime)
		entryBuf[8] &= 0x0F

		if _, err = w.Write(entryBuf[:]); err != nil {
			return
		}
		n += 12
	}
	return
}

func (box *SegmentIndexBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 8 {
		return nil, io.ErrShortBuffer
	}
	n := 0
	box.ReferenceID = binary.BigEndian.Uint32(buf[n:])
	n += 4
	box.TimeScale = binary.BigEndian.Uint32(buf[n:])
	n += 4

	if box.Version == 0 {
		if len(buf) < n+8 {
			return nil, io.ErrShortBuffer
		}
		box.EarliestPresentationTime = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		box.FirstOffset = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	} else {
		if len(buf) < n+16 {
			return nil, io.ErrShortBuffer
		}
		box.EarliestPresentationTime = binary.BigEndian.Uint64(buf[n:])
		n += 8
		box.FirstOffset = binary.BigEndian.Uint64(buf[n:])
		n += 8
	}

	if len(buf) < n+4 {
		return nil, io.ErrShortBuffer
	}
	n += 2 // skip reserved
	box.ReferenceCount = binary.BigEndian.Uint16(buf[n:])
	n += 2

	if len(buf) < n+int(box.ReferenceCount)*12 {
		return nil, io.ErrShortBuffer
	}
	box.Entries = make([]SidxEntry, box.ReferenceCount)
	for i := uint16(0); i < box.ReferenceCount; i++ {
		box.Entries[i].ReferenceType = buf[n] >> 7
		buf[n] = buf[n] & 0x7F
		box.Entries[i].ReferencedSize = binary.BigEndian.Uint32(buf[n:])
		n += 4
		box.Entries[i].SubsegmentDuration = binary.BigEndian.Uint32(buf[n:])
		n += 4
		box.Entries[i].StartsWithSAP = buf[n] >> 7
		box.Entries[i].SAPType = buf[n] >> 4 & 0x07
		buf[n] = buf[n] & 0x0F
		box.Entries[i].SAPDeltaTime = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	return box, nil
}

func init() {
	RegisterBox[*SegmentIndexBox](TypeSIDX)
}
