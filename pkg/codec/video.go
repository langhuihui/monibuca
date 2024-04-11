package codec

import "encoding/binary"

type FourCC [4]byte

var (
	FourCC_H264 = FourCC{'a', 'v', 'c', '1'}
	FourCC_H265 = FourCC{'h', 'v', 'c', '1'}
	FourCC_AV1  = FourCC{'a', 'v', '0', '1'}
	FourCC_VP9  = FourCC{'v', 'p', '0', '9'}
	FourCC_VP8  = FourCC{'v', 'p', '8', '0'}
	FourCC_MP4A = FourCC{'m', 'p', '4', 'a'}
	FourCC_OPUS = FourCC{'O', 'p', 'u', 's'}
	FourCC_ALAW = FourCC{'a', 'l', 'a', 'w'}
	FourCC_ULAW = FourCC{'u', 'l', 'a', 'w'}
)

func (f *FourCC) String() string {
	return string(f[:])
}

func (f *FourCC) Uint32() uint32 {
	return binary.BigEndian.Uint32(f[:])
}

type SPSInfo struct {
	ProfileIdc uint
	LevelIdc   uint

	MbWidth  uint
	MbHeight uint

	CropLeft   uint
	CropRight  uint
	CropTop    uint
	CropBottom uint

	Width  uint
	Height uint
}
