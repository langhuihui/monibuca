package box

import (
	"bytes"
	"encoding/binary"
	"io"
)

const (
	BasicBoxLen = 8
	FullBoxLen  = 12
)

var (
	TypeFTYP = [4]byte{'f', 't', 'y', 'p'}
	TypeSTYP = [4]byte{'s', 't', 'y', 'p'}
	TypeMOOV = [4]byte{'m', 'o', 'o', 'v'}
	TypeMVHD = [4]byte{'m', 'v', 'h', 'd'}
	TypeTRAK = [4]byte{'t', 'r', 'a', 'k'}
	TypeTKHD = [4]byte{'t', 'k', 'h', 'd'}
	TypeMDIA = [4]byte{'m', 'd', 'i', 'a'}
	TypeMDHD = [4]byte{'m', 'd', 'h', 'd'}
	TypeHDLR = [4]byte{'h', 'd', 'l', 'r'}
	TypeMINF = [4]byte{'m', 'i', 'n', 'f'}
	TypeSTBL = [4]byte{'s', 't', 'b', 'l'}
	TypeSTSD = [4]byte{'s', 't', 's', 'd'}
	TypeSTTS = [4]byte{'s', 't', 't', 's'}
	TypeSTSC = [4]byte{'s', 't', 's', 'c'}
	TypeSTSZ = [4]byte{'s', 't', 's', 'z'}
	TypeSTCO = [4]byte{'s', 't', 'c', 'o'}
	TypeMDAT = [4]byte{'m', 'd', 'a', 't'}
	TypeFREE = [4]byte{'f', 'r', 'e', 'e'}
	TypeUUID = [4]byte{'u', 'u', 'i', 'd'}

	TypeVMHD = [4]byte{'v', 'm', 'h', 'd'}
	TypeSMHD = [4]byte{'s', 'm', 'h', 'd'}
	TypeHMHD = [4]byte{'h', 'm', 'h', 'd'}
	TypeNMHD = [4]byte{'n', 'm', 'h', 'd'}
	TypeCTTS = [4]byte{'c', 't', 't', 's'}
	TypeCO64 = [4]byte{'c', 'o', '6', '4'}
	TypePSSH = [4]byte{'p', 's', 's', 'h'}

	TypeSTSS = [4]byte{'s', 't', 's', 's'}
	TypeENCv = [4]byte{'e', 'n', 'c', 'v'}
	TypeSINF = [4]byte{'s', 'i', 'n', 'f'}
	TypeFRMA = [4]byte{'f', 'r', 'm', 'a'}
	TypeSCHI = [4]byte{'s', 'c', 'h', 'i'}
	TypeTENC = [4]byte{'t', 'e', 'n', 'c'}
	TypeAVC1 = [4]byte{'a', 'v', 'c', '1'}
	TypeHVC1 = [4]byte{'h', 'v', 'c', '1'}
	TypeHEV1 = [4]byte{'h', 'e', 'v', '1'}
	TypeENCA = [4]byte{'e', 'n', 'c', 'a'}
	TypeMP4A = [4]byte{'m', 'p', '4', 'a'}
	TypeULAW = [4]byte{'u', 'l', 'a', 'w'}
	TypeALAW = [4]byte{'a', 'l', 'a', 'w'}
	TypeOPUS = [4]byte{'o', 'p', 'u', 's'}
	TypeAVCC = [4]byte{'a', 'v', 'c', 'C'}
	TypeHVCC = [4]byte{'h', 'v', 'c', 'C'}
	TypeESDS = [4]byte{'e', 's', 'd', 's'}
	TypeEDTS = [4]byte{'e', 'd', 't', 's'}
	TypeELST = [4]byte{'e', 'l', 's', 't'}
	TypeMVEX = [4]byte{'m', 'v', 'e', 'x'}
	TypeMOOF = [4]byte{'m', 'o', 'o', 'f'}
	TypeMFHD = [4]byte{'m', 'f', 'h', 'd'}
	TypeTRAF = [4]byte{'t', 'r', 'a', 'f'}
	TypeTFHD = [4]byte{'t', 'f', 'h', 'd'}
	TypeTFDT = [4]byte{'t', 'f', 'd', 't'}
	TypeTRUN = [4]byte{'t', 'r', 'u', 'n'}
	TypeSENC = [4]byte{'s', 'e', 'n', 'c'}
	TypeSAIZ = [4]byte{'s', 'a', 'i', 'z'}
	TypeSAIO = [4]byte{'s', 'a', 'i', 'o'}
	TypeSGPD = [4]byte{'s', 'g', 'p', 'd'}
	TypeWAVE = [4]byte{'w', 'a', 'v', 'e'}
	TypeMSDH = [4]byte{'m', 's', 'd', 'h'}
	TypeMSIX = [4]byte{'m', 's', 'i', 'x'}
	TypeISOM = [4]byte{'i', 's', 'o', 'm'}
	TypeISO2 = [4]byte{'i', 's', 'o', '2'}
	TypeISO3 = [4]byte{'i', 's', 'o', '3'}
	TypeISO4 = [4]byte{'i', 's', 'o', '4'}
	TypeISO5 = [4]byte{'i', 's', 'o', '5'}
	TypeISO6 = [4]byte{'i', 's', 'o', '6'}
	TypeMP41 = [4]byte{'m', 'p', '4', '1'}
	TypeMP42 = [4]byte{'m', 'p', '4', '2'}
	TypeDASH = [4]byte{'d', 'a', 's', 'h'}
	TypeMFRA = [4]byte{'m', 'f', 'r', 'a'}
)

type BoxEncoder interface {
	Encode(buf []byte) (int, []byte)
}

type BoxDecoder interface {
	Decode(buf []byte) (int, error)
}

type BoxSize interface {
	Size() uint64
}

// aligned(8) class Box (unsigned int(32) boxtype, optional unsigned int(8)[16] extended_type) {
//     unsigned int(32) size;
//     unsigned int(32) type = boxtype;
//     if (size==1) {
//        unsigned int(64) largesize;
//     } else if (size==0) {
//        // box extends to end of file
//     }
//     if (boxtype=='uuid') {
//     unsigned int(8)[16] usertype = extended_type;
//  }
// }

type BasicBox struct {
	Size     uint64
	Type     [4]byte
	UserType [16]byte
}

func NewBasicBox(boxtype [4]byte) *BasicBox {
	return &BasicBox{
		Type: boxtype,
	}
}

func (box *BasicBox) Decode(r io.Reader) (int, error) {
	buf := make([]byte, 16)
	if n, err := io.ReadFull(r, buf[:8]); err != nil {
		return n, err
	}
	boxsize := binary.BigEndian.Uint32(buf)
	copy(box.Type[:], buf[4:8])
	nn := 8
	if boxsize == 1 {
		if n, err := io.ReadFull(r, buf[8:]); err != nil {
			return n, err
		}
		box.Size = binary.BigEndian.Uint64(buf[nn:])
		nn += 8
	} else {
		box.Size = uint64(boxsize)
	}
	if bytes.Equal(box.Type[:], []byte("uuid")) {
		uuid := make([]byte, 16)
		if n, err := io.ReadFull(r, uuid); err != nil {
			return n + nn, err
		}
		copy(box.UserType[:], uuid[:])
		nn += 16
	}

	return nn, nil
}

func (box *BasicBox) Encode() (int, []byte) {
	nn := 8
	var buf []byte
	if box.Size > 0xFFFFFFFF { //just for mdat box
		buf = make([]byte, 16)
		binary.BigEndian.PutUint32(buf, 1)
		copy(buf[4:], box.Type[:])
		nn += 8
		binary.BigEndian.PutUint64(buf[8:], box.Size)
	} else {
		buf = make([]byte, box.Size)
		binary.BigEndian.PutUint32(buf, uint32(box.Size))
		copy(buf[4:], box.Type[:])
		if bytes.Equal(box.Type[:], []byte("uuid")) {
			copy(buf[nn:nn+16], box.UserType[:])
			nn += 16
		}
	}
	return nn, buf
}

// aligned(8) class FullBox(unsigned int(32) boxtype, unsigned int(8) v, bit(24) f) extends Box(boxtype) {
//     unsigned int(8) version = v;
//     bit(24) flags = f;
// }

type FullBox struct {
	Box     *BasicBox
	Version uint8
	Flags   [3]byte
}

func NewFullBox(boxtype [4]byte, version uint8) *FullBox {
	return &FullBox{
		Box:     NewBasicBox(boxtype),
		Version: version,
	}
}

func (box *FullBox) Size() uint64 {
	if box.Box.Size > 0 {
		return box.Box.Size
	} else {
		return 12
	}
}

func (box *FullBox) Decode(r io.Reader) (int, error) {
	buf := make([]byte, 4)
	if n, err := io.ReadFull(r, buf); err != nil {
		return n, err
	}
	box.Version = buf[0]
	copy(box.Flags[:], buf[1:])
	return 4, nil
}

func (box *FullBox) Encode() (int, []byte) {
	box.Box.Size = box.Size()
	offset, buf := box.Box.Encode()
	buf[offset] = box.Version
	copy(buf[offset+1:], box.Flags[:])
	return offset + 4, buf
}
