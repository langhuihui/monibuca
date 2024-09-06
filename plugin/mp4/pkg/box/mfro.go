package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class MovieFragmentRandomAccessOffsetBox extends FullBox(‘mfro’, version, 0) {
// 	unsigned int(32)  size;
// }

type MovieFragmentRandomAccessOffsetBox struct {
	Box        *FullBox
	SizeOfMfra uint32
}

func NewMovieFragmentRandomAccessOffsetBox(size uint32) *MovieFragmentRandomAccessOffsetBox {
	return &MovieFragmentRandomAccessOffsetBox{
		Box:        NewFullBox([4]byte{'m', 'f', 'r', 'o'}, 0),
		SizeOfMfra: size,
	}
}

func (mfro *MovieFragmentRandomAccessOffsetBox) Size() uint64 {
	return mfro.Box.Size() + 4
}

func (mfro *MovieFragmentRandomAccessOffsetBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = mfro.Box.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	mfro.SizeOfMfra = binary.BigEndian.Uint32(buf)
	return offset + 4, nil
}

func (mfro *MovieFragmentRandomAccessOffsetBox) Encode() (int, []byte) {
	mfro.Box.Box.Size = mfro.Size()
	offset, boxdata := mfro.Box.Encode()
	binary.BigEndian.PutUint32(boxdata[offset:], mfro.SizeOfMfra)
	return offset + 4, boxdata
}

func makeMfroBox(mfraSize uint32) []byte {
	mfro := NewMovieFragmentRandomAccessOffsetBox(mfraSize + 16)
	_, boxData := mfro.Encode()
	return boxData
}
