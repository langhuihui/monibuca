package box

import (
	"io"
)

type FreeBox struct {
	Box  BasicBox
	Data []byte
}

func NewFreeBox() *FreeBox {
	return &FreeBox{
		Box: BasicBox{
			Type: [4]byte{'f', 'r', 'e', 'e'},
		},
	}
}

func (free *FreeBox) Size() uint64 {
	return 8 + uint64(len(free.Data))
}

func (free *FreeBox) Decode(r io.Reader) (int, error) {
	if BasicBoxLen < free.Box.Size {
		free.Data = make([]byte, free.Box.Size-BasicBoxLen)
		if _, err := io.ReadFull(r, free.Data); err != nil {
			return 0, err
		}
	}
	return int(free.Box.Size - BasicBoxLen), nil
}

func (free *FreeBox) Encode() (int, []byte) {
	free.Box.Size = free.Size()
	offset, buf := free.Box.Encode()
	copy(buf[offset:], free.Data)
	return int(free.Box.Size), buf
}

func decodeFreeBox(demuxer *MovDemuxer, size uint32) (err error) {
	var free FreeBox
	free.Box.Size = uint64(size)
	_, err = free.Decode(demuxer.reader)
	return
}
