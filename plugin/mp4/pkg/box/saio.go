package box

import (
	"encoding/binary"
	"io"
)

// SaioBox - Sample Auxiliary Information Offsets Box (saiz) (in stbl or traf box)
type SaioBox struct {
	AuxInfoType          string // Used for Common Encryption Scheme (4-bytes uint32 according to spec)
	AuxInfoTypeParameter uint32
	Offset               []int64
}

func (s *SaioBox) Decode(r io.Reader, size uint32) error {
	var fullbox FullBox
	if _, err := fullbox.Decode(r); err != nil {
		return err
	}
	buf := make([]byte, size-12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	var n int
	flags := uint32(fullbox.Flags[0])<<16 | uint32(fullbox.Flags[1])<<8 | uint32(fullbox.Flags[2])
	if flags&0x01 != 0 {
		s.AuxInfoType = string(buf[n : n+4])
		n += 4
		s.AuxInfoTypeParameter = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	entryCount := binary.BigEndian.Uint32(buf[n:])
	n += 4
	if fullbox.Version == 0 {
		for i := uint32(0); i < entryCount; i++ {
			s.Offset = append(s.Offset, int64(binary.BigEndian.Uint32(buf[n:])))
			n += 4
		}
	} else {
		for i := uint32(0); i < entryCount; i++ {
			s.Offset = append(s.Offset, int64(binary.BigEndian.Uint64(buf[n:])))
			n += 8
		}
	}
	return nil
}
