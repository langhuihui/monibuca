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
	Box     *FullBox
	Balance int16
}

func NewSoundMediaHeaderBox() *SoundMediaHeaderBox {
	return &SoundMediaHeaderBox{
		Box: NewFullBox([4]byte{'s', 'm', 'h', 'd'}, 0),
	}
}

func (smhd *SoundMediaHeaderBox) Size() uint64 {
	return smhd.Box.Size() + 4
}

func (smhd *SoundMediaHeaderBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = smhd.Box.Decode(r); err != nil {
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
	smhd.Box.Box.Size = smhd.Size()
	offset, buf := smhd.Box.Encode()
	binary.BigEndian.PutUint16(buf[offset:], uint16(smhd.Balance))
	return offset + 2, buf
}

func makeSmhdBox() []byte {
	smhd := NewSoundMediaHeaderBox()
	_, smhdbox := smhd.Encode()
	return smhdbox
}

func decodeSmhdBox(demuxer *MovDemuxer) (err error) {
	smhd := SoundMediaHeaderBox{Box: new(FullBox)}
	_, err = smhd.Decode(demuxer.reader)
	return
}
