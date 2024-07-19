package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class HintMediaHeaderBox
//    extends FullBox(‘hmhd’, version = 0, 0) {
//    unsigned int(16)  maxPDUsize;
//    unsigned int(16)  avgPDUsize;
//    unsigned int(32)  maxbitrate;
//    unsigned int(32)  avgbitrate;
//    unsigned int(32)  reserved = 0;
// }

type HintMediaHeaderBox struct {
	Box        *FullBox
	MaxPDUsize uint16
	AvgPDUsize uint16
	Maxbitrate uint32
	Avgbitrate uint32
}

func NewHintMediaHeaderBox() *HintMediaHeaderBox {
	return &HintMediaHeaderBox{
		Box: NewFullBox([4]byte{'h', 'm', 'h', 'd'}, 0),
	}
}

func (hmhd *HintMediaHeaderBox) Size() uint64 {
	return hmhd.Box.Size() + 16
}

func (hmhd *HintMediaHeaderBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = hmhd.Box.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, 16)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	offset = 0
	hmhd.MaxPDUsize = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	hmhd.AvgPDUsize = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	hmhd.Maxbitrate = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	hmhd.Avgbitrate = binary.BigEndian.Uint32(buf[offset:])
	offset += 8
	return
}

func (hmhd *HintMediaHeaderBox) Encode() (int, []byte) {
	hmhd.Box.Box.Size = hmhd.Size()
	offset, buf := hmhd.Box.Encode()
	binary.BigEndian.PutUint16(buf[offset:], hmhd.MaxPDUsize)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], hmhd.AvgPDUsize)
	offset += 2
	binary.BigEndian.PutUint32(buf[offset:], hmhd.Maxbitrate)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], hmhd.Avgbitrate)
	offset += 8
	return offset, buf
}

func makeHmhdBox() []byte {
	hmhd := NewHintMediaHeaderBox()
	_, hmhdbox := hmhd.Encode()
	return hmhdbox
}
