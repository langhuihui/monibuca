package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class MovieFragmentRandomAccessOffsetBox extends FullBox('mfro', version, 0) {
// 	unsigned int(32)  size;
// }

type MovieFragmentRandomAccessOffsetBox struct {
	FullBox
	MfraSize uint32
}

func CreateMfroBox(mfraSize uint32) *MovieFragmentRandomAccessOffsetBox {
	return &MovieFragmentRandomAccessOffsetBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeMFRO,
				size: uint32(FullBoxLen + 4),
			},
		},
		MfraSize: mfraSize,
	}
}

func (box *MovieFragmentRandomAccessOffsetBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], box.MfraSize)
	var nn int
	nn, err = w.Write(tmp[:])
	if err != nil {
		return 0, err
	}
	return int64(nn), nil
}

func (box *MovieFragmentRandomAccessOffsetBox) Unmarshal(buf []byte) (IBox, error) {
	box.MfraSize = binary.BigEndian.Uint32(buf)
	return box, nil
}

func init() {
	RegisterBox[*MovieFragmentRandomAccessOffsetBox](TypeMFRO)
}
