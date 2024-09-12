package box

import (
	"encoding/binary"
	"io"
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
	Pre_defined  uint32
	Handler_type HandlerType
	Reserved     [3]uint32
	Name         string
}

func NewHandlerBox(handlerType HandlerType, name string) *HandlerBox {
	return &HandlerBox{
		Handler_type: handlerType,
		Name:         name,
	}
}

func (hdlr *HandlerBox) Decode(r io.Reader, size uint64) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, size-FullBoxLen)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	hdlr.Pre_defined = binary.BigEndian.Uint32(buf[:4])
	copy(hdlr.Handler_type[:], buf[4:8])
	hdlr.Name = string(buf[20 : size-FullBoxLen])
	offset = int(size - FullBoxLen)
	return
}

func (hdlr *HandlerBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeHDLR, 0)
	fullbox.Box.Size = 20 + uint64(len(hdlr.Name)+1) + FullBoxLen
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], hdlr.Pre_defined)
	copy(buf[offset+4:], hdlr.Handler_type[:])
	offset += 20
	copy(buf[offset:], []byte(hdlr.Name))
	return offset + len(hdlr.Name), buf
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

func MakeHdlrBox(hdt HandlerType) []byte {
	var hdlr *HandlerBox = nil
	if hdt == TypeVIDE {
		hdlr = NewHandlerBox(hdt, "VideoHandler")
	} else if hdt == TypeSOUN {
		hdlr = NewHandlerBox(hdt, "SoundHandler")
	} else {
		hdlr = NewHandlerBox(hdt, "")
	}
	_, boxdata := hdlr.Encode()
	return boxdata
}
