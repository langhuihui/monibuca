package box

import (
	"encoding/binary"

	"github.com/yapingcat/gomedia/go-codec"
)

// abstract aligned(8) expandable(228-1) class BaseDescriptor : bit(8) tag=0 {
// 	// empty. To be filled by classes extending this class.
// }

//  int sizeOfInstance = 0;
// 	bit(1) nextByte;
// 	bit(7) sizeOfInstance;
// 	while(nextByte) {
// 		bit(1) nextByte;
// 		bit(7) sizeByte;
// 		sizeOfInstance = sizeOfInstance<<7 | sizeByte;
// }

type BaseDescriptor struct {
	tag            uint8
	sizeOfInstance uint32
}

func (base *BaseDescriptor) Decode(data []byte) *codec.BitStream {
	bs := codec.NewBitStream(data)
	base.tag = bs.Uint8(8)
	nextbit := uint8(1)
	for nextbit == 1 {
		nextbit = bs.GetBit()
		base.sizeOfInstance = base.sizeOfInstance<<7 | bs.Uint32(7)
	}
	return bs
}

func (base *BaseDescriptor) Encode() []byte {
	bsw := codec.NewBitStreamWriter(5 + int(base.sizeOfInstance))
	bsw.PutByte(base.tag)
	size := base.sizeOfInstance
	bsw.PutUint8(1, 1)
	bsw.PutUint8(uint8(size>>21), 7)
	bsw.PutUint8(1, 1)
	bsw.PutUint8(uint8(size>>14), 7)
	bsw.PutUint8(1, 1)
	bsw.PutUint8(uint8(size>>7), 7)
	bsw.PutUint8(0, 1)
	bsw.PutUint8(uint8(size), 7)
	return bsw.Bits()[:5+int(base.sizeOfInstance)]
}

// ffmpeg mov_write_esds_tag
func makeBaseDescriptor(tag uint8, size uint32) []byte {
	base := BaseDescriptor{
		tag:            tag,
		sizeOfInstance: size,
	}
	return base.Encode()
}

func makeESDescriptor(trackid uint16, cid MP4_CODEC_TYPE, vosData []byte) []byte {
	dcd := makeDecoderConfigDescriptor(cid, vosData)
	sld := makeSLDescriptor()
	esd := makeBaseDescriptor(0x03, uint32(len(dcd)+len(sld)+3))
	binary.BigEndian.PutUint16(esd[5:], trackid)
	esd[7] = 0x00
	copy(esd[8:], dcd)
	copy(esd[8+len(dcd):], sld)
	return esd
}

func makeDecoderConfigDescriptor(cid MP4_CODEC_TYPE, vosData []byte) []byte {

	decoder_specific_info_len := uint32(0)
	if len(vosData) > 0 {
		decoder_specific_info_len = uint32(len(vosData)) + 5
	}
	dcd := makeBaseDescriptor(0x04, 13+decoder_specific_info_len)
	dcd[5] = getBojecttypeWithCodecId(cid)
	if cid == MP4_CODEC_H264 || cid == MP4_CODEC_H265 {
		dcd[6] = 0x11
	} else if cid == MP4_CODEC_G711A || cid == MP4_CODEC_G711U || cid == MP4_CODEC_AAC {
		dcd[6] = 0x15
	} else {
		dcd[6] = (0x38 << 2) | 1
	}
	dcd[7] = 0
	dcd[8] = 0
	dcd[9] = 0
	binary.BigEndian.PutUint32(dcd[10:], 88360)
	binary.BigEndian.PutUint32(dcd[14:], 88360)
	if decoder_specific_info_len > 0 {
		dsd := makeDecoderSpecificInfoDescriptor(vosData)
		copy(dcd[18:], dsd)
	}
	return dcd
}

func makeDecoderSpecificInfoDescriptor(vosData []byte) []byte {
	dsd := makeBaseDescriptor(0x05, uint32(len(vosData)))
	copy(dsd[5:], vosData)
	return dsd
}

func makeSLDescriptor() []byte {
	sldes := makeBaseDescriptor(0x06, 1)
	sldes[5] = 0x02
	return sldes
}

func decodeESDescriptor(esd []byte, track *mp4track) (vosData []byte) {
	var bs *codec.BitStream
	for len(esd) > 0 {
		based := BaseDescriptor{}
		bs = based.Decode(esd)
		switch based.tag {
		case 0x03:
			_ = bs.Uint8(16) // esId
			streamDependenceFlag := bs.Uint8(1)
			_ = bs.Uint8(1) // urlFlag
			oCRstreamFlag := bs.Uint8(1)
			_ = bs.Uint8(5) //streamPriority
			if streamDependenceFlag == 1 {
				_ = bs.Uint8(16) // dependsOnEsId
			}
			if oCRstreamFlag == 1 {
				_ = bs.Uint8(16) // oCREsId
			}
			esd = bs.RemainData()
		case 0x04:
			track.cid = getCodecIdByObjectType(bs.Uint8(8))
			bs.Uint8(32)
			bs.Uint8(64)
			esd = bs.RemainData()
		case 0x05:
			vosData = bs.GetBytes(int(based.sizeOfInstance))
			esd = bs.RemainData()
		case 0x06:
			fallthrough
		default:
			bs.SkipBits(int(based.sizeOfInstance) * 8)
			esd = bs.RemainData()
		}
	}
	if track.cid == MP4_CODEC_AAC && len(vosData) == 0 {
		panic("no vosdata")
	}
	if track.cid == MP4_CODEC_AAC {
		track.extra = new(aacExtraData)
	}
	return
}
