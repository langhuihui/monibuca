package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackRunBox extends FullBox('trun', version, tr_flags) {
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

type TrunEntry struct {
	SampleDuration              uint32
	SampleSize                  uint32
	SampleFlags                 uint32
	SampleCompositionTimeOffset int32
}

type TrackRunBox struct {
	FullBox
	SampleCount      uint32
	DataOffset       int32
	FirstSampleFlags uint32
	Entries          []TrunEntry
}

func CreateTrackRunBox(flags uint32, sampleCount uint32) *TrackRunBox {
	size := uint32(FullBoxLen + 4) // base size + sample_count

	if flags&TR_FLAG_DATA_OFFSET != 0 {
		size += 4
	}
	if flags&TR_FLAG_DATA_FIRST_SAMPLE_FLAGS != 0 {
		size += 4
	}

	entrySize := uint32(0)
	if flags&TR_FLAG_DATA_SAMPLE_DURATION != 0 {
		entrySize += 4
	}
	if flags&TR_FLAG_DATA_SAMPLE_SIZE != 0 {
		entrySize += 4
	}
	if flags&TR_FLAG_DATA_SAMPLE_FLAGS != 0 {
		entrySize += 4
	}
	if flags&TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME != 0 {
		entrySize += 4
	}

	size += entrySize * sampleCount

	return &TrackRunBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTRUN,
				size: size,
			},
			Version: 1, // Use version 1 for signed composition time offsets
			Flags:   [3]byte{byte(flags >> 16), byte(flags >> 8), byte(flags)},
		},
		SampleCount: sampleCount,
		Entries:     make([]TrunEntry, sampleCount),
	}
}

func (box *TrackRunBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], box.SampleCount)
	nn, err := w.Write(tmp[:])
	if err != nil {
		return int64(nn), err
	}
	n = int64(nn)

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])

	if flags&TR_FLAG_DATA_OFFSET != 0 {
		binary.BigEndian.PutUint32(tmp[:], uint32(box.DataOffset))
		nn, err = w.Write(tmp[:])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	if flags&TR_FLAG_DATA_FIRST_SAMPLE_FLAGS != 0 {
		binary.BigEndian.PutUint32(tmp[:], box.FirstSampleFlags)
		nn, err = w.Write(tmp[:])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	for i := uint32(0); i < box.SampleCount; i++ {
		if flags&TR_FLAG_DATA_SAMPLE_DURATION != 0 {
			binary.BigEndian.PutUint32(tmp[:], box.Entries[i].SampleDuration)
			nn, err = w.Write(tmp[:])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)
		}

		if flags&TR_FLAG_DATA_SAMPLE_SIZE != 0 {
			binary.BigEndian.PutUint32(tmp[:], box.Entries[i].SampleSize)
			nn, err = w.Write(tmp[:])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)
		}

		if flags&TR_FLAG_DATA_SAMPLE_FLAGS != 0 {
			binary.BigEndian.PutUint32(tmp[:], box.Entries[i].SampleFlags)
			nn, err = w.Write(tmp[:])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)
		}

		if flags&TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME != 0 {
			binary.BigEndian.PutUint32(tmp[:], uint32(box.Entries[i].SampleCompositionTimeOffset))
			nn, err = w.Write(tmp[:])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)
		}
	}

	return
}

func (box *TrackRunBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}

	n := 0
	box.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])

	if flags&TR_FLAG_DATA_OFFSET != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.DataOffset = int32(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	}

	if flags&TR_FLAG_DATA_FIRST_SAMPLE_FLAGS != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.FirstSampleFlags = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	box.Entries = make([]TrunEntry, box.SampleCount)
	for i := uint32(0); i < box.SampleCount; i++ {
		if flags&TR_FLAG_DATA_SAMPLE_DURATION != 0 {
			if len(buf) < n+4 {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].SampleDuration = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}

		if flags&TR_FLAG_DATA_SAMPLE_SIZE != 0 {
			if len(buf) < n+4 {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].SampleSize = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}

		if flags&TR_FLAG_DATA_SAMPLE_FLAGS != 0 {
			if len(buf) < n+4 {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].SampleFlags = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}

		if flags&TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME != 0 {
			if len(buf) < n+4 {
				return nil, io.ErrShortBuffer
			}
			if box.Version == 0 {
				box.Entries[i].SampleCompositionTimeOffset = int32(binary.BigEndian.Uint32(buf[n:]))
			} else {
				box.Entries[i].SampleCompositionTimeOffset = int32(binary.BigEndian.Uint32(buf[n:]))
			}
			n += 4
		}
	}

	return box, nil
}

func init() {
	RegisterBox[*TrackRunBox](TypeTRUN)
}
