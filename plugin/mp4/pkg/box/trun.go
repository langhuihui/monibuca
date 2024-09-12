package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackRunBox extends FullBox(‘trun’, version, tr_flags) {
//      unsigned int(32) sample_count;
//      // the following are optional fields
//      signed int(32) data_offset;
//       unsigned int(32) first_sample_flags;
//      // all fields in the following array are optional
//      {
//          unsigned int(32) sample_duration;
//          unsigned int(32) sample_size;
//          unsigned int(32) sample_flags
//          if (version == 0)
//          {
//              unsigned int(32) sample_composition_time_offset;
//          }
//          else
//          {
//              signed int(32) sample_composition_time_offset;
//          }
//      }[ sample_count ]
// }

const (
	TR_FLAG_DATA_OFFSET                  uint32 = 0x000001
	TR_FLAG_DATA_FIRST_SAMPLE_FLAGS      uint32 = 0x000004
	TR_FLAG_DATA_SAMPLE_DURATION         uint32 = 0x000100
	TR_FLAG_DATA_SAMPLE_SIZE             uint32 = 0x000200
	TR_FLAG_DATA_SAMPLE_FLAGS            uint32 = 0x000400
	TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME uint32 = 0x000800
)

type TrackRunBox struct {
	SampleCount      uint32
	Dataoffset       int32
	FirstSampleFlags uint32
	EntryList        []TrunEntry
}

func NewTrackRunBox() *TrackRunBox {
	return &TrackRunBox{}
}

func (trun *TrackRunBox) Size(trunFlags uint32) uint64 {
	n := uint64(FullBoxLen)
	n += 4
	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		n += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		n += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	return n
}

func (trun *TrackRunBox) Decode(r io.Reader, size uint32, dataOffset uint32) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	trun.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trunFlags := uint32(fullbox.Flags[0])<<16 | uint32(fullbox.Flags[1])<<8 | uint32(fullbox.Flags[2])

	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		trun.Dataoffset = int32(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	} else {
		trun.Dataoffset = int32(dataOffset)
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		trun.FirstSampleFlags = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	trun.EntryList = make([]TrunEntry, trun.SampleCount)
	for i := 0; i < int(trun.SampleCount); i++ {
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) > 0 {
			trun.EntryList[i].SampleDuration = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) > 0 {
			trun.EntryList[i].SampleSize = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) > 0 {
			trun.EntryList[i].SampleFlags = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) > 0 {
			trun.EntryList[i].SampleCompositionTimeOffset = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
	}
	offset += n
	return
}

func (trun *TrackRunBox) Encode(trunFlags uint32) (int, []byte) {
	fullbox := NewFullBox(TypeTRUN, 1)
	fullbox.Box.Size = trun.Size(trunFlags)
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], trun.SampleCount)
	offset += 4

	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], uint32(trun.Dataoffset))
		offset += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], trun.FirstSampleFlags)
		offset += 4
	}

	for i := 0; i < int(trun.SampleCount); i++ {
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList[i].SampleDuration)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList[i].SampleSize)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList[i].SampleFlags)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList[i].SampleCompositionTimeOffset)
			offset += 4
		}
	}
	return offset, buf
}
