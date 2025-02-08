package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackFragmentHeaderBox extends FullBox('tfhd', 0, tf_flags){
//     unsigned int(32) track_ID;
//     // all the following are optional fields
//     unsigned int(64) base_data_offset;
//     unsigned int(32) sample_description_index;
//     unsigned int(32) default_sample_duration;
//     unsigned int(32) default_sample_size;
//     unsigned int(32) default_sample_flags
// }

const (
	TF_FLAG_BASE_DATA_OFFSET_PRESENT         uint32 = 0x000001
	TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT uint32 = 0x000002
	TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT  uint32 = 0x000008
	TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT      uint32 = 0x000010
	TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT     uint32 = 0x000020
	TF_FLAG_DURATION_IS_EMPTY                uint32 = 0x010000
	TF_FLAG_DEFAULT_BASE_IS_MOOF             uint32 = 0x020000

	// Sample flags
	MOV_FRAG_SAMPLE_FLAG_DEGRADATION_PRIORITY_MASK uint32 = 0x0000ffff
	MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC               uint32 = 0x10000000
	MOV_FRAG_SAMPLE_FLAG_IS_SYNC                   uint32 = 0x00000000
	MOV_FRAG_SAMPLE_FLAG_PADDING_MASK              uint32 = 0x000e0000
	MOV_FRAG_SAMPLE_FLAG_REDUNDANCY_MASK           uint32 = 0x00300000
	MOV_FRAG_SAMPLE_FLAG_DEPENDED_MASK             uint32 = 0x00c00000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_MASK              uint32 = 0x03000000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_RESERVED          uint32 = 0x03000000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO                uint32 = 0x02000000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES               uint32 = 0x01000000
)

type TrackFragmentHeaderBox struct {
	FullBox
	TrackID                uint32
	BaseDataOffset         uint64
	SampleDescriptionIndex uint32
	DefaultSampleDuration  uint32
	DefaultSampleSize      uint32
	DefaultSampleFlags     uint32
}

func CreateTrackFragmentHeaderBox(trackID uint32, flags uint32) *TrackFragmentHeaderBox {
	size := uint32(FullBoxLen + 4) // base size + track_ID
	if flags&TF_FLAG_BASE_DATA_OFFSET_PRESENT != 0 {
		size += 8
	}
	if flags&TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT != 0 {
		size += 4
	}
	if flags&TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT != 0 {
		size += 4
	}
	if flags&TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT != 0 {
		size += 4
	}
	if flags&TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT != 0 {
		size += 4
	}

	return &TrackFragmentHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTFHD,
				size: size,
			},
			Version: 0,
			Flags:   [3]byte{byte(flags >> 16), byte(flags >> 8), byte(flags)},
		},
		TrackID:                trackID,
		SampleDescriptionIndex: 1,
	}
}

func (box *TrackFragmentHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	binary.BigEndian.PutUint32(tmp[:], box.TrackID)
	nn, err := w.Write(tmp[:4])
	if err != nil {
		return int64(nn), err
	}
	n = int64(nn)

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])

	if flags&TF_FLAG_BASE_DATA_OFFSET_PRESENT != 0 {
		binary.BigEndian.PutUint64(tmp[:], box.BaseDataOffset)
		nn, err = w.Write(tmp[:8])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	if flags&TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT != 0 {
		binary.BigEndian.PutUint32(tmp[:], box.SampleDescriptionIndex)
		nn, err = w.Write(tmp[:4])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT != 0 {
		binary.BigEndian.PutUint32(tmp[:], box.DefaultSampleDuration)
		nn, err = w.Write(tmp[:4])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT != 0 {
		binary.BigEndian.PutUint32(tmp[:], box.DefaultSampleSize)
		nn, err = w.Write(tmp[:4])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT != 0 {
		binary.BigEndian.PutUint32(tmp[:], box.DefaultSampleFlags)
		nn, err = w.Write(tmp[:4])
		if err != nil {
			return n + int64(nn), err
		}
		n += int64(nn)
	}

	return
}

func (box *TrackFragmentHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}

	n := 0
	box.TrackID = binary.BigEndian.Uint32(buf[n:])
	n += 4

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])

	if flags&TF_FLAG_BASE_DATA_OFFSET_PRESENT != 0 {
		if len(buf) < n+8 {
			return nil, io.ErrShortBuffer
		}
		box.BaseDataOffset = binary.BigEndian.Uint64(buf[n:])
		n += 8
	}

	if flags&TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.SampleDescriptionIndex = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.DefaultSampleDuration = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.DefaultSampleSize = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	if flags&TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT != 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		box.DefaultSampleFlags = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	return box, nil
}

func init() {
	RegisterBox[*TrackFragmentHeaderBox](TypeTFHD)
}
