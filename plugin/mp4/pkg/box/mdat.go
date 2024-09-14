package box

import (
	"encoding/binary"
)

type MediaDataBox uint64

func (box MediaDataBox) Encode() (int, []byte) {
	if box+BasicBoxLen > 0xFFFFFFFF {
		basicBox := NewBasicBox(TypeMDAT)
		basicBox.Size = BasicBoxLen + 8 + uint64(box)
		buf := make([]byte, BasicBoxLen*2)
		binary.BigEndian.PutUint32(buf, uint32(1))
		copy(buf[4:], basicBox.Type[:])
		binary.BigEndian.PutUint64(buf[8:], uint64(box))
		return BasicBoxLen * 2, buf
	} else {
		basicBox := NewBasicBox(TypeMDAT)
		basicBox.Size = BasicBoxLen + uint64(box)
		buf := make([]byte, BasicBoxLen)
		binary.BigEndian.PutUint32(buf, uint32(basicBox.Size))
		copy(buf[4:], basicBox.Type[:])
		return BasicBoxLen, buf
	}
}
