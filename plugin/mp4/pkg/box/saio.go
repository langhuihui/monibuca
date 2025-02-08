package box

import (
	"encoding/binary"
	"io"
)

// SaioBox - Sample Auxiliary Information Offsets Box (saiz) (in stbl or traf box)
type SaioBox struct {
	FullBox
	AuxInfoType          string // Used for Common Encryption Scheme (4-bytes uint32 according to spec)
	AuxInfoTypeParameter uint32
	Offset               []int64
}

func CreateSaioBox(auxInfoType string, auxInfoTypeParameter uint32, offset []int64) *SaioBox {
	flags := uint32(0)
	size := uint32(FullBoxLen + 4) // base size + entry_count
	if len(auxInfoType) > 0 {
		flags |= 0x01
		size += 8 // auxInfoType(4) + auxInfoTypeParameter(4)
	}
	if len(offset) > 0 {
		if offset[0] > 0xFFFFFFFF {
			size += uint32(len(offset) * 8)
		} else {
			size += uint32(len(offset) * 4)
		}
	}
	return &SaioBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSAIO,
				size: size,
			},
			Version: 0,
			Flags:   [3]byte{byte(flags >> 16), byte(flags >> 8), byte(flags)},
		},
		AuxInfoType:          auxInfoType,
		AuxInfoTypeParameter: auxInfoTypeParameter,
		Offset:               offset,
	}
}

func (box *SaioBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])

	if flags&0x01 != 0 {
		copy(tmp[:4], []byte(box.AuxInfoType))
		if _, err = w.Write(tmp[:4]); err != nil {
			return
		}
		binary.BigEndian.PutUint32(tmp[:4], box.AuxInfoTypeParameter)
		if _, err = w.Write(tmp[:4]); err != nil {
			return
		}
		n += 8
	}

	binary.BigEndian.PutUint32(tmp[:4], uint32(len(box.Offset)))
	if _, err = w.Write(tmp[:4]); err != nil {
		return
	}
	n += 4

	for _, offset := range box.Offset {
		if box.Version == 0 {
			binary.BigEndian.PutUint32(tmp[:4], uint32(offset))
			if _, err = w.Write(tmp[:4]); err != nil {
				return
			}
			n += 4
		} else {
			binary.BigEndian.PutUint64(tmp[:], uint64(offset))
			if _, err = w.Write(tmp[:]); err != nil {
				return
			}
			n += 8
		}
	}
	return
}

func (box *SaioBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}
	n := 0
	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])
	if flags&0x01 != 0 {
		if len(buf) < n+8 {
			return nil, io.ErrShortBuffer
		}
		box.AuxInfoType = string(buf[n : n+4])
		n += 4
		box.AuxInfoTypeParameter = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	entryCount := binary.BigEndian.Uint32(buf[n:])
	n += 4

	box.Offset = make([]int64, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		if box.Version == 0 {
			if len(buf) < n+4 {
				return nil, io.ErrShortBuffer
			}
			box.Offset[i] = int64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
		} else {
			if len(buf) < n+8 {
				return nil, io.ErrShortBuffer
			}
			box.Offset[i] = int64(binary.BigEndian.Uint64(buf[n:]))
			n += 8
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*SaioBox](TypeSAIO)
}
