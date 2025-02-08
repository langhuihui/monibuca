package box

import (
	"encoding/binary"
	"io"
	"net"
)

type FileTypeBox struct {
	BaseBox
	MajorBrand       BoxType
	MinorVersion     uint32
	CompatibleBrands []BoxType
}

func CreateFTYPBox(major BoxType, minor uint32, compatibleBrands ...BoxType) *FileTypeBox {
	return &FileTypeBox{
		BaseBox: BaseBox{
			typ:  TypeFTYP,
			size: uint32(BasicBoxLen + len(compatibleBrands)*4 + 8),
		},
		MajorBrand:       major,
		MinorVersion:     minor,
		CompatibleBrands: compatibleBrands,
	}
}

func (box *FileTypeBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	buffers := make(net.Buffers, 0, len(box.CompatibleBrands)+2)
	binary.BigEndian.PutUint32(tmp[:], box.MinorVersion)
	buffers = append(buffers, box.MajorBrand[:], tmp[:])
	for _, brand := range box.CompatibleBrands {
		buffers = append(buffers, brand[:])
	}
	return buffers.WriteTo(w)
}

func (box *FileTypeBox) Unmarshal(buf []byte) (IBox, error) {
	box.MajorBrand = BoxType(buf[:4])
	box.MinorVersion = binary.BigEndian.Uint32(buf[4:8])
	for i := 8; i < len(buf); i += 4 {
		box.CompatibleBrands = append(box.CompatibleBrands, BoxType(buf[i:i+4]))
	}
	return box, nil
}

func init() {
	RegisterBox[*FileTypeBox](TypeFTYP, TypeSTYP)
}
