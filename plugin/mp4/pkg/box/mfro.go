package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class MovieFragmentRandomAccessOffsetBox extends FullBox(‘mfro’, version, 0) {
// 	unsigned int(32)  size;
// }

type MovieFragmentRandomAccessOffsetBox uint32

func (mfro *MovieFragmentRandomAccessOffsetBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	*mfro = MovieFragmentRandomAccessOffsetBox(binary.BigEndian.Uint32(buf))
	return offset + 4, nil
}

func (mfro *MovieFragmentRandomAccessOffsetBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeMFRO, 0)
	fullbox.Box.Size = FullBoxLen + 4
	offset, boxdata := fullbox.Encode()
	binary.BigEndian.PutUint32(boxdata[offset:], uint32(*mfro))
	return offset + 4, boxdata
}

func MakeMfroBox(mfraSize uint32) []byte {
	mfro := MovieFragmentRandomAccessOffsetBox(mfraSize + 16)
	_, boxData := mfro.Encode()
	return boxData
}
