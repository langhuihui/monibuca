package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackFragmentHeaderBox extends FullBox(‘tfhd’, 0, tf_flags){
//     unsigned int(32) track_ID;
//     // all the following are optional fields
//     unsigned int(64) base_data_offset;
//     unsigned int(32) sample_description_index;
//     unsigned int(32) default_sample_duration;
//     unsigned int(32) default_sample_size;
//     unsigned int(32) default_sample_flags
// }

const (
	TF_FLAG_BASE_DATA_OFFSET                 uint32 = 0x000001
	TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT uint32 = 0x000002
	TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT  uint32 = 0x000008
	TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT      uint32 = 0x000010
	TF_FLAG_DEAAULT_SAMPLE_FLAGS_PRESENT     uint32 = 0x000020
	TF_FLAG_DURATION_IS_EMPTY                uint32 = 0x010000
	TF_FLAG_DEAAULT_BASE_IS_MOOF             uint32 = 0x020000

	//ffmpeg isom.h
	MOV_FRAG_SAMPLE_FLAG_DEGRADATION_PRIORITY_MASK uint32 = 0x0000ffff
	MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC               uint32 = 0x00010000
	MOV_FRAG_SAMPLE_FLAG_PADDING_MASK              uint32 = 0x000e0000
	MOV_FRAG_SAMPLE_FLAG_REDUNDANCY_MASK           uint32 = 0x00300000
	MOV_FRAG_SAMPLE_FLAG_DEPENDED_MASK             uint32 = 0x00c00000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_MASK              uint32 = 0x03000000

	MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO  uint32 = 0x02000000
	MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES uint32 = 0x01000000
)

type TrackFragmentHeaderBox struct {
	Track_ID               uint32
	BaseDataOffset         uint64
	SampleDescriptionIndex uint32
	DefaultSampleDuration  uint32
	DefaultSampleSize      uint32
	DefaultSampleFlags     uint32
}

func NewTrackFragmentHeaderBox(trackid uint32) *TrackFragmentHeaderBox {
	return &TrackFragmentHeaderBox{
		Track_ID:               trackid,
		SampleDescriptionIndex: 1,
	}
}

func (tfhd *TrackFragmentHeaderBox) Size(thfdFlags uint32) uint64 {
	n := uint64(FullBoxLen)
	n += 4
	if thfdFlags&TF_FLAG_BASE_DATA_OFFSET > 0 {
		n += 8
	}
	if thfdFlags&TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT > 0 {
		n += 4
	}
	if thfdFlags&TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT > 0 {
		n += 4
	}
	if thfdFlags&TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT > 0 {
		n += 4
	}
	if thfdFlags&TF_FLAG_DEAAULT_SAMPLE_FLAGS_PRESENT > 0 {
		n += 4
	}
	return n
}

func (tfhd *TrackFragmentHeaderBox) Decode(r io.Reader, size uint32, moofOffset uint64) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	tfhd.Track_ID = binary.BigEndian.Uint32(buf[n:])
	n += 4
	tfhdFlags := uint32(fullbox.Flags[0])<<16 | uint32(fullbox.Flags[1])<<8 | uint32(fullbox.Flags[2])
	if tfhdFlags&uint32(TF_FLAG_BASE_DATA_OFFSET) > 0 {
		tfhd.BaseDataOffset = binary.BigEndian.Uint64(buf[n:])
		n += 8
	} else if tfhdFlags&uint32(TF_FLAG_DEAAULT_BASE_IS_MOOF) > 0 {
		tfhd.BaseDataOffset = moofOffset
	} else {
		//TODO,In some cases, it is wrong
		tfhd.BaseDataOffset = moofOffset
	}

	if tfhdFlags&uint32(TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT) > 0 {
		tfhd.SampleDescriptionIndex = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	if tfhdFlags&uint32(TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT) > 0 {
		tfhd.DefaultSampleDuration = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	if tfhdFlags&uint32(TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT) > 0 {
		tfhd.DefaultSampleSize = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	if tfhdFlags&uint32(TF_FLAG_DEAAULT_SAMPLE_FLAGS_PRESENT) > 0 {
		tfhd.DefaultSampleFlags = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	offset += n
	return
}

func (tfhd *TrackFragmentHeaderBox) Encode(tFfFlags uint32) (int, []byte) {
	fullbox := NewFullBox(TypeTFHD, 0)
	fullbox.Flags[0] = byte(tFfFlags >> 16)
	fullbox.Flags[1] = byte(tFfFlags >> 8)
	fullbox.Flags[2] = byte(tFfFlags)
	fullbox.Box.Size = tfhd.Size(tFfFlags)
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], tfhd.Track_ID)
	offset += 4
	thfdFlags := uint32(fullbox.Flags[0])<<16 | uint32(fullbox.Flags[1])<<8 | uint32(fullbox.Flags[2])
	if thfdFlags&uint32(TF_FLAG_BASE_DATA_OFFSET) > 0 {
		binary.BigEndian.PutUint64(buf[offset:], tfhd.BaseDataOffset)
		offset += 8
	}
	if thfdFlags&uint32(TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], tfhd.SampleDescriptionIndex)
		offset += 4
	}
	if thfdFlags&uint32(TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], tfhd.DefaultSampleDuration)
		offset += 4
	}
	if thfdFlags&uint32(TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], tfhd.DefaultSampleSize)
		offset += 4
	}
	if thfdFlags&uint32(TF_FLAG_DEAAULT_SAMPLE_FLAGS_PRESENT) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], tfhd.DefaultSampleFlags)
		offset += 4
	}
	return offset, buf
}
