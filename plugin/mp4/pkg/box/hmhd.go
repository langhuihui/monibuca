package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class HintMediaHeaderBox
//    extends FullBox('hmhd', version = 0, 0) {
//    unsigned int(16)  maxPDUsize;
//    unsigned int(16)  avgPDUsize;
//    unsigned int(32)  maxbitrate;
//    unsigned int(32)  avgbitrate;
//    unsigned int(32)  reserved = 0;
// }

type HintMediaHeaderBox struct {
	FullBox
	MaxPDUsize uint16
	AvgPDUsize uint16
	Maxbitrate uint32
	Avgbitrate uint32
}

func CreateHintMediaHeaderBox() *HintMediaHeaderBox {
	return &HintMediaHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeHMHD,
				size: uint32(FullBoxLen + 16),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
	}
}

func (box *HintMediaHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [16]byte
	binary.BigEndian.PutUint16(tmp[0:], box.MaxPDUsize)
	binary.BigEndian.PutUint16(tmp[2:], box.AvgPDUsize)
	binary.BigEndian.PutUint32(tmp[4:], box.Maxbitrate)
	binary.BigEndian.PutUint32(tmp[8:], box.Avgbitrate)
	// reserved already zeroed

	nn, err := w.Write(tmp[:])
	n = int64(nn)
	return
}

func (box *HintMediaHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 16 {
		return nil, io.ErrShortBuffer
	}
	box.MaxPDUsize = binary.BigEndian.Uint16(buf[0:])
	box.AvgPDUsize = binary.BigEndian.Uint16(buf[2:])
	box.Maxbitrate = binary.BigEndian.Uint32(buf[4:])
	box.Avgbitrate = binary.BigEndian.Uint32(buf[8:])
	return box, nil
}

func init() {
	RegisterBox[*HintMediaHeaderBox](TypeHMHD)
}
