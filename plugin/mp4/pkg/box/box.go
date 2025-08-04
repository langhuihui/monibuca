package box

import (
	"encoding/binary"
	"io"
	"net"
	"reflect"
	"unsafe"

	"m7s.live/v5/pkg/util"
)

type (
	BoxType [4]byte

	BoxHeader interface {
		Type() BoxType
		HeaderSize() uint32
		Size() uint64
		Header() BoxHeader
		HeaderWriteTo(w io.Writer) (n int64, err error)
	}

	IBox interface {
		BoxHeader
		io.WriterTo
		Unmarshal(buf []byte) (IBox, error)
	}

	// 基础Box结构，实现通用字段
	BaseBox struct {
		typ  BoxType
		size uint32
	}
	DataBox struct {
		BaseBox
		Data []byte
	}
	MemoryBox struct {
		BaseBox
		Data util.Memory
	}
	BigBox struct {
		BaseBox
		size uint64
	}

	FullBox struct {
		BaseBox
		Version uint8
		Flags   [3]byte
	}
	ContainerBox struct {
		BaseBox
		Children []IBox
	}
)

func CreateBaseBox(typ BoxType, size uint64) IBox {
	if size > 0xFFFFFFFF {
		return &BigBox{
			BaseBox: BaseBox{
				typ:  typ,
				size: 1,
			},
			size: size + 8,
		}
	}

	return &BaseBox{
		typ:  typ,
		size: uint32(size),
	}
}

func CreateDataBox(typ BoxType, data []byte) *DataBox {
	return &DataBox{
		BaseBox: BaseBox{
			typ:  typ,
			size: uint32(len(data)) + BasicBoxLen,
		},
		Data: data,
	}
}

func CreateMemoryBox(typ BoxType, mem util.Memory) *MemoryBox {
	return &MemoryBox{
		BaseBox: BaseBox{
			typ:  typ,
			size: uint32(mem.Size) + BasicBoxLen,
		},
		Data: mem,
	}
}

func CreateContainerBox(typ BoxType, children ...IBox) *ContainerBox {
	size := uint32(BasicBoxLen)
	realChildren := make([]IBox, 0, len(children))
	for _, child := range children {
		if reflect.ValueOf(child).IsNil() {
			continue
		}
		size += uint32(child.Size())
		realChildren = append(realChildren, child)
	}
	return &ContainerBox{
		BaseBox: BaseBox{
			typ:  typ,
			size: size,
		},
		Children: realChildren,
	}
}

func (b *BigBox) HeaderSize() uint32 { return BasicBoxLen + 8 }

func (b *BaseBox) Header() BoxHeader  { return b }
func (b *BaseBox) HeaderSize() uint32 { return BasicBoxLen }
func (b *BaseBox) Size() uint64       { return uint64(b.size) }
func (b *BigBox) Size() uint64        { return uint64(b.size) }
func (b *BaseBox) Type() BoxType      { return b.typ }

func (b *BaseBox) HeaderWriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], b.size)
	buffers := net.Buffers{tmp[:], b.typ[:]}
	return buffers.WriteTo(w)
}

func (b *BaseBox) WriteTo(w io.Writer) (n int64, err error) {
	return
}

func (b *ContainerBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, b.Children...)
}

func (b *DataBox) WriteTo(w io.Writer) (n int64, err error) {
	_, err = w.Write(b.Data)
	return int64(len(b.Data)), err
}

func (b *BaseBox) Unmarshal(buf []byte) (IBox, error) {
	return b, nil
}

func (b *MemoryBox) WriteTo(w io.Writer) (n int64, err error) {
	return b.Data.WriteTo(w)
}

func (b *MemoryBox) Unmarshal(buf []byte) (IBox, error) {
	b.Data.PushOne(buf)
	return b, nil
}

func (b *DataBox) Unmarshal(buf []byte) (IBox, error) {
	b.Data = buf
	return b, nil
}

func (b *FullBox) HeaderWriteTo(w io.Writer) (n int64, err error) {

	var tmp [4]byte

	binary.BigEndian.PutUint32(tmp[:], b.size)
	buffers := net.Buffers{tmp[:], b.typ[:], []byte{b.Version}, b.Flags[:]}
	return buffers.WriteTo(w)
}

func (b *BigBox) HeaderWriteTo(w io.Writer) (n int64, err error) {
	n, err = b.BaseBox.HeaderWriteTo(w)
	if err != nil {
		return
	}
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], b.size)
	_, err = w.Write(tmp[:])
	return n + 8, err
}

func (b *BigBox) Header() BoxHeader   { return b }
func (b *FullBox) Header() BoxHeader  { return b }
func (b *FullBox) HeaderSize() uint32 { return FullBoxLen }

func WriteTo(w io.Writer, box ...IBox) (n int64, err error) {
	var n1, n2 int64
	for _, b := range box {
		if reflect.ValueOf(b).IsNil() {
			continue
		}
		n1, err = b.HeaderWriteTo(w)
		if err != nil {
			return
		}
		n2, err = b.WriteTo(w)
		if err != nil {
			return
		}
		if n1+n2 != int64(b.Size()) {
			// panic(fmt.Sprintf("write to %s size error, %d != %d", b.Type(), n1+n2, b.Size()))
		}
		n += n1 + n2
	}
	return

}

func ReadFrom(r io.Reader) (box IBox, err error) {
	var tmp [8]byte
	if _, err = io.ReadFull(r, tmp[:]); err != nil {
		return
	}
	var baseBox BaseBox
	baseBox.size = binary.BigEndian.Uint32(tmp[:4])
	baseBox.typ = BoxType(tmp[4:])
	t, exists := registry[baseBox.typ.Uint32I()]
	if !exists {
		io.CopyN(io.Discard, r, int64(baseBox.size-BasicBoxLen))
		return &baseBox, nil
	}
	b := reflect.New(t.Elem()).Interface().(IBox)
	var payload []byte
	if baseBox.size == 1 {
		if _, err = io.ReadFull(r, tmp[:]); err != nil {
			return
		}
		payload = make([]byte, binary.BigEndian.Uint64(tmp[:])-BasicBoxLen-8)
	} else {
		payload = make([]byte, baseBox.size-BasicBoxLen)
	}
	_, err = io.ReadFull(r, payload)
	if err != nil {
		return nil, err
	}
	boxHeader := b.Header()
	switch header := boxHeader.(type) {
	case *BaseBox:
		*header = baseBox
		box, err = b.Unmarshal(payload)
	case *FullBox:
		header.BaseBox = baseBox
		header.Version = payload[0]
		header.Flags = [3]byte(payload[1:4])
		box, err = b.Unmarshal(payload[4:])
	}
	if err == io.EOF {
		return box, nil
	}
	return
}

func (b BoxType) String() string {
	return string(b[:])
}

func (b BoxType) Uint32() uint32 {
	return binary.BigEndian.Uint32(b[:])
}

func (b BoxType) Uint32I() uint32 {
	return *(*uint32)(unsafe.Pointer(&b[0]))
}

var registry = map[uint32]reflect.Type{}

// RegisterBox 注册box类型
func RegisterBox[T any](typ ...BoxType) {
	var b T
	bt := reflect.TypeOf(b)
	for _, t := range typ {
		registry[t.Uint32I()] = bt
	}
}

const (
	BasicBoxLen = 8  // size(4) + type(4)
	FullBoxLen  = 12 // BasicBoxLen + version(1) + flags(3)
)

func f(s string) BoxType {
	return BoxType([]byte(s))
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
	TypeDOPS = f("dOps")
	TypeOPUS = f("opus")
	TypeAVCC = f("avcC")
	TypeHVCC = f("hvcC")
	TypeESDS = f("esds")
	TypeEDTS = f("edts")
	TypeELST = f("elst")
	TypeMVEX = f("mvex")
	TypeMEHD = f("mehd")
	TypeMOOF = f("moof")
	TypeMFHD = f("mfhd")
	TypeTRAF = f("traf")
	TypeTFHD = f("tfhd")
	TypeTFDT = f("tfdt")
	TypeTRUN = f("trun")
	TypeSDTP = f("sdtp")
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
	TypeUDTA = f("udta")

	// Common metadata box types
	TypeTITL      = f("©nam") // Title
	TypeART       = f("©ART") // Artist/Author
	TypeALB       = f("©alb") // Album
	TypeDAY       = f("©day") // Date/Year
	TypeCMT       = f("©cmt") // Comment/Description
	TypeGEN       = f("©gen") // Genre
	TypeCPRT      = f("cprt") // Copyright
	TypeENCO      = f("©too") // Encoder/Tool
	TypeWRT       = f("©wrt") // Writer/Composer
	TypePRD       = f("©prd") // Producer
	TypePRF       = f("©prf") // Performer
	TypeGRP       = f("©grp") // Grouping
	TypeLYR       = f("©lyr") // Lyrics
	TypeKEYW      = f("keyw") // Keywords
	TypeLOCI      = f("loci") // Location Information
	TypeRTNG      = f("rtng") // Rating
	TypeMETA_CUST = f("----") // Custom metadata (iTunes-style)
)

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

// aligned(8) class FullBox(unsigned int(32) boxtype, unsigned int(8) v, bit(24) f) extends Box(boxtype) {
//     unsigned int(8) version = v;
//     bit(24) flags = f;
// }

type TimeToSampleEntry struct {
	SampleCount uint32
	SampleDelta uint32
}

type CompositionTimeToSampleEntry struct {
	SampleCount  uint32
	SampleOffset int32
}

type SampleToChunkEntry struct {
	FirstChunk             uint32
	SamplesPerChunk        uint32
	SampleDescriptionIndex uint32
}

func ConvertUnixTimeToISO14496(unixTime uint64) uint64 {
	return unixTime + 0x7C25B080
}
