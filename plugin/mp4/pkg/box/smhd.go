package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SoundMediaHeaderBox
//    extends FullBox(‘smhd’, version = 0, 0) {
//    template int(16) balance = 0;
//    const unsigned int(16)  reserved = 0;
// }

type SoundMediaHeaderBox struct {
	Balance int16
}

func NewSoundMediaHeaderBox() *SoundMediaHeaderBox {
	return &SoundMediaHeaderBox{}
}

func (smhd *SoundMediaHeaderBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	smhd.Balance = int16(binary.BigEndian.Uint16(buf[:]))
	return 4, nil
}

func (smhd *SoundMediaHeaderBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeSMHD, 0)
	fullbox.Box.Size = FullBoxLen + 4
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint16(buf[offset:], uint16(smhd.Balance))
	return offset + 2, buf
}

func MakeSmhdBox() []byte {
	smhd := NewSoundMediaHeaderBox()
	_, smhdbox := smhd.Encode()
	return smhdbox
}
