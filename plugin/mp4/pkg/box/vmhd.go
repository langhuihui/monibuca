package box

import (
	"encoding/binary"
	"io"
)

// Box Types: 'vmhd', 'smhd', 'hmhd', 'nmhd'
// Container: Media Information Box ('minf')
// Mandatory: Yes
// Quantity: Exactly one specific media header shall be present

// aligned(8) class VideoMediaHeaderBox
// extends FullBox('vmhd', version = 0, 1) {
// template unsigned int(16) graphicsmode = 0; // copy, see below template
// unsigned int(16)[3] opcolor = {0, 0, 0};
// }

type VideoMediaHeaderBox struct {
	FullBox
	Graphicsmode uint16
	Opcolor      [3]uint16
}

func CreateVideoMediaHeaderBox() *VideoMediaHeaderBox {
	return &VideoMediaHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeVMHD,
				size: uint32(FullBoxLen + 8),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 1}, // Flags = 1
		},
		Graphicsmode: 0,
		Opcolor:      [3]uint16{0, 0, 0},
	}
}

func (box *VideoMediaHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	binary.BigEndian.PutUint16(tmp[0:], box.Graphicsmode)
	binary.BigEndian.PutUint16(tmp[2:], box.Opcolor[0])
	binary.BigEndian.PutUint16(tmp[4:], box.Opcolor[1])
	binary.BigEndian.PutUint16(tmp[6:], box.Opcolor[2])

	nn, err := w.Write(tmp[:])
	n = int64(nn)
	return
}

func (box *VideoMediaHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 8 {
		return nil, io.ErrShortBuffer
	}
	box.Graphicsmode = binary.BigEndian.Uint16(buf[0:])
	box.Opcolor[0] = binary.BigEndian.Uint16(buf[2:])
	box.Opcolor[1] = binary.BigEndian.Uint16(buf[4:])
	box.Opcolor[2] = binary.BigEndian.Uint16(buf[6:])
	return box, nil
}

func init() {
	RegisterBox[*VideoMediaHeaderBox](TypeVMHD)
}
