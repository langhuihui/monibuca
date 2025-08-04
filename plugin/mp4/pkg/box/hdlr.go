package box

import (
	"encoding/binary"
	"io"
	"net"
)

// Box Type: 'hdlr'
// Container: Media Box (‘mdia’) or Meta Box (‘meta’)
// Mandatory: Yes
// Quantity: Exactly one

// aligned(8) class HandlerBox extends FullBox(‘hdlr’, version = 0, 0) {
//  unsigned int(32) pre_defined = 0;
// 	unsigned int(32) handler_type;
// 	const unsigned int(32)[3] reserved = 0;
// 	   string   name;
// 	}

// handler_type
// value from a derived specification:
// ‘vide’ Video track
// ‘soun’ Audio track
// ‘hint’ Hint track
// ‘meta’ Timed Metadata track
// ‘auxv’ Auxiliary Video track

type HandlerType = [4]byte
type HandlerBox struct {
	FullBox
	Pre_defined  uint32
	Handler_type HandlerType
	Name         string
}

func NewHandlerBox(handlerType HandlerType, name string) *HandlerBox {
	return &HandlerBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeHDLR,
				size: uint32(20 + len(name) + 1 + FullBoxLen),
			},
		},
		Handler_type: handlerType,
		Name:         name,
	}
}

func (hdlr *HandlerBox) Unmarshal(buf []byte) (IBox, error) {
	hdlr.Pre_defined = binary.BigEndian.Uint32(buf[:4])
	copy(hdlr.Handler_type[:], buf[4:8])
	hdlr.Name = string(buf[20:])
	return hdlr, nil
}

func (hdlr *HandlerBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [20]byte
	binary.BigEndian.PutUint32(tmp[:], hdlr.Pre_defined)
	copy(tmp[4:8], hdlr.Handler_type[:])
	var buffer = net.Buffers{
		tmp[:],
		[]byte(hdlr.Name),
		[]byte{0},
	}
	return buffer.WriteTo(w)

}

func GetHandlerType(cid MP4_CODEC_TYPE) HandlerType {
	switch cid {
	case MP4_CODEC_H264, MP4_CODEC_H265:
		return TypeVIDE
	case MP4_CODEC_AAC, MP4_CODEC_G711A, MP4_CODEC_G711U,
		MP4_CODEC_MP2, MP4_CODEC_MP3, MP4_CODEC_OPUS:
		return TypeSOUN
	default:
		panic("unsupport codec id")
	}
}

func MakeHdlrBox(hdt HandlerType) *HandlerBox {
	var hdlr *HandlerBox = nil
	switch hdt {
	case TypeVIDE:
		hdlr = NewHandlerBox(hdt, "VideoHandler")
	case TypeSOUN:
		hdlr = NewHandlerBox(hdt, "SoundHandler")
	default:
		hdlr = NewHandlerBox(hdt, "")
	}
	return hdlr
}

func init() {
	RegisterBox[*HandlerBox](TypeHDLR)
}
