package box

import (
	"encoding/binary"
	"io"
)

// Box Types: ‘vmhd’, ‘smhd’, ’hmhd’, ‘nmhd’
// Container: Media Information Box (‘minf’)
// Mandatory: Yes
// Quantity: Exactly one specific media header shall be present

// aligned(8) class VideoMediaHeaderBox
// extends FullBox(‘vmhd’, version = 0, 1) {
// template unsigned int(16) graphicsmode = 0; // copy, see below template
// unsigned int(16)[3] opcolor = {0, 0, 0};
// }

type VideoMediaHeaderBox struct {
	Graphicsmode uint16
	Opcolor      [3]uint16
}

func NewVideoMediaHeaderBox() *VideoMediaHeaderBox {
	return &VideoMediaHeaderBox{
		Graphicsmode: 0,
		Opcolor:      [3]uint16{0, 0, 0},
	}
}

func (vmhd *VideoMediaHeaderBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, 8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	offset = 0
	vmhd.Graphicsmode = binary.BigEndian.Uint16(buf[offset:])
	vmhd.Opcolor[0] = binary.BigEndian.Uint16(buf[offset+2:])
	vmhd.Opcolor[1] = binary.BigEndian.Uint16(buf[offset+4:])
	vmhd.Opcolor[2] = binary.BigEndian.Uint16(buf[offset+6:])
	offset += 8
	return
}

func (vmhd *VideoMediaHeaderBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeVMHD, 0)
	fullbox.Box.Size = FullBoxLen + 8
	fullbox.Flags[2] = 1
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint16(buf[offset:], vmhd.Graphicsmode)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], vmhd.Opcolor[0])
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], vmhd.Opcolor[1])
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], vmhd.Opcolor[2])
	offset += 2
	return offset, buf
}

func MakeVmhdBox() []byte {
	vmhd := NewVideoMediaHeaderBox()
	_, vmhdbox := vmhd.Encode()
	return vmhdbox
}
