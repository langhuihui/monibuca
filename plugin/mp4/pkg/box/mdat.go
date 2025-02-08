package box

import (
	"encoding/binary"
	"io"
	"net"
)

// aligned(8) class MediaDataBox extends Box('mdat') {
//     bit(8) data[];
// }

type MediaDataBox struct {
	BaseBox
	Data []byte
	net.Buffers
}

func (box *MediaDataBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	var buffers net.Buffers
	if box.size == 1 {
		// 写入扩展大小头部
		binary.BigEndian.PutUint64(tmp[:], uint64(len(box.Data))+BasicBoxLen+8) // largesize
		buffers = append(buffers, tmp[:])
	}
	if box.Data != nil {
		buffers = append(buffers, box.Data)
	}
	if box.Buffers != nil {
		buffers = append(buffers, box.Buffers...)
	}
	return buffers.WriteTo(w)
}

func (box *MediaDataBox) Unmarshal(buf []byte) (IBox, error) {
	box.Data = buf
	return box, nil
}

func init() {
	RegisterBox[*MediaDataBox](TypeMDAT)
}
