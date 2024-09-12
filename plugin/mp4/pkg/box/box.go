package box

import (
	"encoding/binary"
	"io"
)

const (
	BasicBoxLen = 8
	FullBoxLen  = 12
)

func f(s string) [4]byte {
	return [4]byte([]byte(s))
}

var (
	TypeFTYP = f("ftyp")
	TypeSTYP = f("styp")
	TypeMOOV = f("moov")
	TypeMVHD = f("mvhd")
	TypeTRAK = f("trak")
	TypeTKHD = f("tkhd")
	TypeMDIA = f("mdia")
	TypeMDHD = f("mdhd")
	TypeHDLR = f("hdlr")
	TypeMINF = f("minf")
	TypeSTBL = f("stbl")
	TypeSTSD = f("stsd")
	TypeSTTS = f("stts")
	TypeSTSC = f("stsc")
	TypeSTSZ = f("stsz")
	TypeSTCO = f("stco")
	TypeMDAT = f("mdat")
	TypeFREE = f("free")
	TypeUUID = f("uuid")

	TypeVMHD = f("vmhd")
	TypeSMHD = f("smhd")
	TypeHMHD = f("hmhd")
	TypeNMHD = f("nmhd")
	TypeCTTS = f("ctts")
	TypeCO64 = f("co64")
	TypePSSH = f("pssh")

	TypeSTSS = f("stss")
	TypeENCV = f("encv")
	TypeSINF = f("sinf")
	TypeFRMA = f("frma")
	TypeSCHI = f("schi")
	TypeTENC = f("tenc")
	TypeAVC1 = f("avc1")
	TypeHVC1 = f("hvc1")
	TypeHEV1 = f("hev1")
	TypeENCA = f("enca")
	TypeMP4A = f("mp4a")
	TypeULAW = f("ulaw")
	TypeALAW = f("alaw")
	TypeOPUS = f("opus")
	TypeAVCC = f("avcC")
	TypeHVCC = f("hvcC")
	TypeESDS = f("esds")
	TypeEDTS = f("edts")
	TypeELST = f("elst")
	TypeMVEX = f("mvex")
	TypeMOOF = f("moof")
	TypeMFHD = f("mfhd")
	TypeTRAF = f("traf")
	TypeTFHD = f("tfhd")
	TypeTFDT = f("tfdt")
	TypeTRUN = f("trun")
	TypeSENC = f("senc")
	TypeSAIZ = f("saiz")
	TypeSAIO = f("saio")
	TypeSGPD = f("sgpd")
	TypeWAVE = f("wave")
	TypeMSDH = f("msdh")
	TypeMSIX = f("msix")
	TypeISOM = f("isom")
	TypeISO2 = f("iso2")
	TypeISO3 = f("iso3")
	TypeISO4 = f("iso4")
	TypeISO5 = f("iso5")
	TypeISO6 = f("iso6")
	TypeMP41 = f("mp41")
	TypeMP42 = f("mp42")
	TypeDASH = f("dash")
	TypeMFRA = f("mfra")
	TypeMFRO = f("mfro")
	TypeTREX = f("trex")
	TypeTFRA = f("tfra")
	TypeSIDX = f("sidx")
	TypeDINF = f("dinf")
	TypeDREF = f("dref")
	TypeVIDE = f("vide")
	TypeSOUN = f("soun")
	TypeMETA = f("meta")
	TypeAUXV = f("auxv")
	TypeHINT = f("hint")
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

//	aligned(8) class Box (unsigned int(32) boxtype, optional unsigned int(8)[16] extended_type) {
//	    unsigned int(32) size;
//	    unsigned int(32) type = boxtype;
//	    if (size==1) {
//	       unsigned int(64) largesize;
//	    } else if (size==0) {
//	       // box extends to end of file
//	    }
//	    if (boxtype=='uuid') {
//	    unsigned int(8)[16] usertype = extended_type;
//	 }
//	}
type IBox interface {
	Decode(io.Reader, *BasicBox) (int, error)
}
type BasicBox struct {
	Offset   int64
	Size     uint64
	Type     [4]byte
	UserType [16]byte
}

func NewBasicBox(boxtype [4]byte) *BasicBox {
	return &BasicBox{
		Type: boxtype,
	}
}

func (box *BasicBox) Decode(r io.Reader) (nn int, err error) {
	if _, err = io.ReadFull(r, box.Type[:]); err != nil {
		return
	}
	box.Size = uint64(binary.BigEndian.Uint32(box.Type[:]))
	if _, err = io.ReadFull(r, box.Type[:]); err != nil {
		return
	}
	nn = BasicBoxLen
	if box.Size == 1 {
		if _, err = io.ReadFull(r, box.UserType[:8]); err != nil {
			return
		}
		box.Size = binary.BigEndian.Uint64(box.UserType[:8])
		box.UserType = [16]byte{}
		nn += 8
	}
	if box.Type == TypeUUID {
		if _, err = io.ReadFull(r, box.UserType[:]); err != nil {
			return
		}
		nn += 16
	}
	return
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
		if box.Type == TypeUUID {
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
		return FullBoxLen
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
