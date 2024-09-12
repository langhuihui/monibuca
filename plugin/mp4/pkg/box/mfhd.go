package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class MovieFragmentHeaderBox extends FullBox(‘mfhd’, 0, 0){
// 	unsigned int(32) sequence_number;
// }

type MovieFragmentHeaderBox uint32

func (mfhd MovieFragmentHeaderBox) Size() uint64 {
	return FullBoxLen + 4
}

func (mfhd *MovieFragmentHeaderBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	*mfhd = MovieFragmentHeaderBox(binary.BigEndian.Uint32(buf))
	return offset + 4, nil
}

func (mfhd MovieFragmentHeaderBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeMFHD, 0)
	fullbox.Box.Size = mfhd.Size()
	offset, boxdata := fullbox.Encode()
	binary.BigEndian.PutUint32(boxdata[offset:], uint32(mfhd))
	return offset + 4, boxdata
}

func MakeMfhdBox(frament uint32) []byte {
	mfhd := MovieFragmentHeaderBox(frament)
	_, boxData := mfhd.Encode()
	return boxData
}
