package box

import (
	"encoding/binary"
	"io"
)

// SaizBox - Sample Auxiliary Information Sizes Box (saiz)  (in stbl or traf box)
type SaizBox struct {
	FullBox
	AuxInfoType           string // Used for Common Encryption Scheme (4-bytes uint32 according to spec)
	AuxInfoTypeParameter  uint32
	DefaultSampleInfoSize uint8
	SampleCount           uint32
	SampleInfo            []byte
}

func CreateSaizBox(auxInfoType string, auxInfoTypeParameter uint32, defaultSampleInfoSize uint8, sampleInfo []byte) *SaizBox {
	flags := uint32(0)
	size := uint32(FullBoxLen + 5) // base size + defaultSampleInfoSize(1) + sampleCount(4)
	if len(auxInfoType) > 0 {
		flags |= 0x01
		size += 8 // auxInfoType(4) + auxInfoTypeParameter(4)
	}
	if defaultSampleInfoSize == 0 {
		size += uint32(len(sampleInfo))
	}
	return &SaizBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSAIZ,
				size: size,
			},
			Version: 0,
			Flags:   [3]byte{byte(flags >> 16), byte(flags >> 8), byte(flags)},
		},
		AuxInfoType:           auxInfoType,
		AuxInfoTypeParameter:  auxInfoTypeParameter,
		DefaultSampleInfoSize: defaultSampleInfoSize,
		SampleCount:           uint32(len(sampleInfo)),
		SampleInfo:            sampleInfo,
	}
}

func (box *SaizBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
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

	if _, err = w.Write([]byte{box.DefaultSampleInfoSize}); err != nil {
		return
	}
	n++

	binary.BigEndian.PutUint32(tmp[:4], box.SampleCount)
	if _, err = w.Write(tmp[:4]); err != nil {
		return
	}
	n += 4

	if box.DefaultSampleInfoSize == 0 && len(box.SampleInfo) > 0 {
		nn, err := w.Write(box.SampleInfo)
		if err != nil {
			return n, err
		}
		n += int64(nn)
	}
	return
}

func (box *SaizBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 5 {
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

	box.DefaultSampleInfoSize = buf[n]
	n++

	box.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4

	if box.DefaultSampleInfoSize == 0 {
		if len(buf) < n+int(box.SampleCount) {
			return nil, io.ErrShortBuffer
		}
		box.SampleInfo = make([]byte, box.SampleCount)
		copy(box.SampleInfo, buf[n:n+int(box.SampleCount)])
	}
	return box, nil
}

func init() {
	RegisterBox[*SaizBox](TypeSAIZ)
}
