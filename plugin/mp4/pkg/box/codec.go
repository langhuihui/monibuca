package box

import (
	"github.com/yapingcat/gomedia/go-codec"
)

type MP4_CODEC_TYPE int

const (
	MP4_CODEC_H264 MP4_CODEC_TYPE = iota + 1
	MP4_CODEC_H265

	MP4_CODEC_AAC MP4_CODEC_TYPE = iota + 100
	MP4_CODEC_G711A
	MP4_CODEC_G711U
	MP4_CODEC_MP2
	MP4_CODEC_MP3
	MP4_CODEC_OPUS
)

func isVideo(cid MP4_CODEC_TYPE) bool {
	return cid == MP4_CODEC_H264 || cid == MP4_CODEC_H265
}

func isAudio(cid MP4_CODEC_TYPE) bool {
	return cid == MP4_CODEC_AAC || cid == MP4_CODEC_G711A || cid == MP4_CODEC_G711U ||
		cid == MP4_CODEC_MP2 || cid == MP4_CODEC_MP3 || cid == MP4_CODEC_OPUS
}

func getCodecNameWithCodecId(cid MP4_CODEC_TYPE) [4]byte {
	switch cid {
	case MP4_CODEC_H264:
		return [4]byte{'a', 'v', 'c', '1'}
	case MP4_CODEC_H265:
		return [4]byte{'h', 'v', 'c', '1'}
	case MP4_CODEC_AAC, MP4_CODEC_MP2, MP4_CODEC_MP3:
		return [4]byte{'m', 'p', '4', 'a'}
	case MP4_CODEC_G711A:
		return [4]byte{'a', 'l', 'a', 'w'}
	case MP4_CODEC_G711U:
		return [4]byte{'u', 'l', 'a', 'w'}
	case MP4_CODEC_OPUS:
		return [4]byte{'o', 'p', 'u', 's'}
	default:
		panic("unsupport codec id")
	}
}

// ffmpeg isom.c const AVCodecTag ff_mp4_obj_type[]
func getBojecttypeWithCodecId(cid MP4_CODEC_TYPE) uint8 {
	switch cid {
	case MP4_CODEC_H264:
		return 0x21
	case MP4_CODEC_H265:
		return 0x23
	case MP4_CODEC_AAC:
		return 0x40
	case MP4_CODEC_G711A:
		return 0xfd
	case MP4_CODEC_G711U:
		return 0xfe
	case MP4_CODEC_MP2:
		return 0x6b
	case MP4_CODEC_MP3:
		return 0x69
	default:
		panic("unsupport codec id")
	}
}

func getCodecIdByObjectType(objType uint8) MP4_CODEC_TYPE {
	switch objType {
	case 0x21:
		return MP4_CODEC_H264
	case 0x23:
		return MP4_CODEC_H265
	case 0x40:
		return MP4_CODEC_AAC
	case 0xfd:
		return MP4_CODEC_G711A
	case 0xfe:
		return MP4_CODEC_G711U
	case 0x6b, 0x69:
		return MP4_CODEC_MP3
	default:
		panic("unsupport object type")
	}
}

func isH264NewAccessUnit(nalu []byte) bool {
	switch codec.H264_NAL_TYPE(nalu[0] & 0x1F) {
	case codec.H264_NAL_AUD, codec.H264_NAL_SPS,
		codec.H264_NAL_PPS, codec.H264_NAL_SEI:
		return true
	case codec.H264_NAL_I_SLICE, codec.H264_NAL_P_SLICE,
		codec.H264_NAL_SLICE_A, codec.H264_NAL_SLICE_B, codec.H264_NAL_SLICE_C:
		firstMbInSlice := GetH264FirstMbInSlice(nalu)
		if firstMbInSlice == 0 {
			return true
		}
	}
	return false
}

func isH265NewAccessUnit(nalu []byte) bool {
	switch codec.H265_NAL_TYPE((nalu[0] >> 1) & 0x3F) {
	case codec.H265_NAL_AUD, codec.H265_NAL_SPS,
		codec.H265_NAL_PPS, codec.H265_NAL_SEI, codec.H265_NAL_VPS:
		return true
	case codec.H265_NAL_Slice_TRAIL_N, codec.H265_NAL_LICE_TRAIL_R,
		codec.H265_NAL_SLICE_TSA_N, codec.H265_NAL_SLICE_TSA_R,
		codec.H265_NAL_SLICE_STSA_N, codec.H265_NAL_SLICE_STSA_R,
		codec.H265_NAL_SLICE_RADL_N, codec.H265_NAL_SLICE_RADL_R,
		codec.H265_NAL_SLICE_RASL_N, codec.H265_NAL_SLICE_RASL_R,
		codec.H265_NAL_SLICE_BLA_W_LP, codec.H265_NAL_SLICE_BLA_W_RADL,
		codec.H265_NAL_SLICE_BLA_N_LP, codec.H265_NAL_SLICE_IDR_W_RADL,
		codec.H265_NAL_SLICE_IDR_N_LP, codec.H265_NAL_SLICE_CRA:
		firstMbInSlice := GetH265FirstMbInSlice(nalu)
		if firstMbInSlice == 0 {
			return true
		}
	}
	return false
}

func GetH264FirstMbInSlice(nalu []byte) uint64 {
	bs := codec.NewBitStream(nalu[1:])
	sliceHdr := &codec.SliceHeader{}
	sliceHdr.Decode(bs)
	return sliceHdr.First_mb_in_slice
}

func GetH265FirstMbInSlice(nalu []byte) uint64 {
	bs := codec.NewBitStream(nalu[2:])
	sliceHdr := &codec.SliceHeader{}
	sliceHdr.Decode(bs)
	return sliceHdr.First_mb_in_slice
}
