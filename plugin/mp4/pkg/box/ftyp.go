package box

import (
	"encoding/binary"
	"io"
)

func mov_tag(tag [4]byte) uint32 {
	return binary.LittleEndian.Uint32(tag[:])
}

type FileTypeBox struct {
	Major_brand       [4]byte
	Minor_version     uint32
	Compatible_brands [][4]byte
}

func (ftyp *FileTypeBox) Decode(r io.Reader, baseBox *BasicBox) (int, error) {
	buf := make([]byte, baseBox.Size-BasicBoxLen)
	if n, err := io.ReadFull(r, buf); err != nil {
		return n, err
	}
	ftyp.Major_brand = [4]byte(buf[0:])
	ftyp.Minor_version = binary.BigEndian.Uint32(buf[4:])
	n := 8
	for ; BasicBoxLen+n < int(baseBox.Size); n += 4 {
		ftyp.Compatible_brands = append(ftyp.Compatible_brands, [4]byte(buf[n:]))
	}
	return n, nil
}

func (ftyp *FileTypeBox) Encode(t [4]byte) (int, []byte) {
	var baseBox BasicBox
	baseBox.Type = t
	baseBox.Size = uint64(BasicBoxLen + len(ftyp.Compatible_brands)*4 + 8)
	offset, buf := baseBox.Encode()
	copy(buf[offset:], ftyp.Major_brand[:])
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], ftyp.Minor_version)
	offset += 4
	for i := 0; offset < int(baseBox.Size); offset += 4 {
		copy(buf[offset:], ftyp.Compatible_brands[i][:])
		i++
	}
	return offset, buf
}

func MakeFtypBox(major [4]byte, minor uint32, compatibleBrands ...[4]byte) []byte {
	var ftyp FileTypeBox
	ftyp.Major_brand = major
	ftyp.Minor_version = minor
	ftyp.Compatible_brands = compatibleBrands
	_, boxData := ftyp.Encode(TypeFTYP)
	return boxData
}

func MakeStypBox(major [4]byte, minor uint32, compatibleBrands ...[4]byte) []byte {
	var styp FileTypeBox
	styp.Major_brand = major
	styp.Minor_version = minor
	styp.Compatible_brands = compatibleBrands
	_, boxData := styp.Encode(TypeSTYP)
	return boxData
}
