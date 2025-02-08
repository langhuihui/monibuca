package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SoundMediaHeaderBox
//    extends FullBox('smhd', version = 0, 0) {
//    template int(16) balance = 0;
//    const unsigned int(16)  reserved = 0;
// }

type SoundMediaHeaderBox struct {
	FullBox
	Balance int16
}

func CreateSoundMediaHeaderBox() *SoundMediaHeaderBox {
	return &SoundMediaHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSMHD,
				size: uint32(FullBoxLen + 4),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		Balance: 0,
	}
}

func (box *SoundMediaHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint16(tmp[0:], uint16(box.Balance))
	// reserved already zeroed

	nn, err := w.Write(tmp[:])
	n = int64(nn)
	return
}

func (box *SoundMediaHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}
	box.Balance = int16(binary.BigEndian.Uint16(buf[0:]))
	return box, nil
}

func init() {
	RegisterBox[*SoundMediaHeaderBox](TypeSMHD)
}
