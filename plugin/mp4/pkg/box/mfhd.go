package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class MovieFragmentHeaderBox extends FullBox('mfhd', 0, 0){
// 	unsigned int(32) sequence_number;
// }

type MovieFragmentHeaderBox struct {
	FullBox
	SequenceNumber uint32
}

func CreateMovieFragmentHeaderBox(sequenceNumber uint32) *MovieFragmentHeaderBox {
	return &MovieFragmentHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeMFHD,
				size: uint32(FullBoxLen + 4),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		SequenceNumber: sequenceNumber,
	}
}

func (box *MovieFragmentHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], box.SequenceNumber)
	nn, err := w.Write(tmp[:])
	n = int64(nn)
	return
}

func (box *MovieFragmentHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}
	box.SequenceNumber = binary.BigEndian.Uint32(buf[:4])
	return box, nil
}

func init() {
	RegisterBox[*MovieFragmentHeaderBox](TypeMFHD)
}
