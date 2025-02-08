package box

import (
	"encoding/binary"
	"io"
)

const (
	UseSubsampleEncryption uint32 = 0x000002
)

// SencBox - Sample Encryption Box (senc) (in trak or traf box)
// See ISO/IEC 23001-7 Section 7.2 and CMAF specification
type SencBox struct {
	FullBox
	SampleCount     uint32
	PerSampleIVSize uint32
	EntryList       []SencEntry
}

func CreateSencBox(perSampleIVSize uint32, entries []SencEntry, useSubSample bool) *SencBox {
	flags := uint32(0)
	if useSubSample {
		flags |= UseSubsampleEncryption
	}

	size := uint32(FullBoxLen + 4) // base size + SampleCount
	for _, entry := range entries {
		size += uint32(len(entry.IV))
		if useSubSample {
			size += 2                                 // subsample count
			size += uint32(len(entry.SubSamples) * 6) // each subsample is 6 bytes
		}
	}

	return &SencBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSENC,
				size: size,
			},
			Version: 0,
			Flags:   [3]byte{byte(flags >> 16), byte(flags >> 8), byte(flags)},
		},
		SampleCount:     uint32(len(entries)),
		PerSampleIVSize: perSampleIVSize,
		EntryList:       entries,
	}
}

func (box *SencBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [6]byte
	binary.BigEndian.PutUint32(tmp[:4], box.SampleCount)
	if _, err = w.Write(tmp[:4]); err != nil {
		return
	}
	n = 4

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])
	for _, entry := range box.EntryList {
		if _, err = w.Write(entry.IV); err != nil {
			return
		}
		n += int64(len(entry.IV))

		if flags&UseSubsampleEncryption != 0 {
			binary.BigEndian.PutUint16(tmp[:2], uint16(len(entry.SubSamples)))
			if _, err = w.Write(tmp[:2]); err != nil {
				return
			}
			n += 2

			for _, subsample := range entry.SubSamples {
				binary.BigEndian.PutUint16(tmp[:2], subsample.BytesOfClearData)
				binary.BigEndian.PutUint32(tmp[2:], subsample.BytesOfProtectedData)
				if _, err = w.Write(tmp[:]); err != nil {
					return
				}
				n += 6
			}
		}
	}
	return
}

func (box *SencBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}
	n := 0
	box.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4

	flags := uint32(box.Flags[0])<<16 | uint32(box.Flags[1])<<8 | uint32(box.Flags[2])
	box.EntryList = make([]SencEntry, box.SampleCount)

	for i := uint32(0); i < box.SampleCount; i++ {
		if len(buf) < n+int(box.PerSampleIVSize) {
			return nil, io.ErrShortBuffer
		}
		box.EntryList[i].IV = make([]byte, box.PerSampleIVSize)
		copy(box.EntryList[i].IV, buf[n:n+int(box.PerSampleIVSize)])
		n += int(box.PerSampleIVSize)

		if flags&UseSubsampleEncryption != 0 {
			if len(buf) < n+2 {
				return nil, io.ErrShortBuffer
			}
			subsampleCount := binary.BigEndian.Uint16(buf[n:])
			n += 2

			if len(buf) < n+int(subsampleCount)*6 {
				return nil, io.ErrShortBuffer
			}
			box.EntryList[i].SubSamples = make([]SubSampleEntry, subsampleCount)
			for j := uint16(0); j < subsampleCount; j++ {
				box.EntryList[i].SubSamples[j].BytesOfClearData = binary.BigEndian.Uint16(buf[n:])
				n += 2
				box.EntryList[i].SubSamples[j].BytesOfProtectedData = binary.BigEndian.Uint32(buf[n:])
				n += 4
			}
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*SencBox](TypeSENC)
}
