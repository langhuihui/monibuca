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
// Full Box + SampleCount
type SencBox struct {
	SampleCount     uint32
	PerSampleIVSize uint32
	EntryList       []SencEntry
}

func (senc *SencBox) Decode(r io.Reader, size uint32, perSampleIVSize uint8) (offset int, err error) {
	var sencBox FullBox
	if offset, err = sencBox.Decode(r); err != nil {
		return
	}
	senc.PerSampleIVSize = uint32(perSampleIVSize)
	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	senc.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4
	sencFlags := uint32(sencBox.Flags[0])<<16 | uint32(sencBox.Flags[1])<<8 | uint32(sencBox.Flags[2])

	senc.EntryList = make([]SencEntry, senc.SampleCount)
	for i := 0; i < int(senc.SampleCount); i++ {
		senc.EntryList[i].IV = buf[n : n+int(senc.PerSampleIVSize)]
		n += int(senc.PerSampleIVSize)

		if sencFlags&UseSubsampleEncryption <= 0 {
			continue
		}

		subsampleCount := binary.BigEndian.Uint16(buf[n:])
		n += 2

		senc.EntryList[i].SubSamples = make([]SubSampleEntry, subsampleCount)
		for j := uint16(0); j < subsampleCount; j++ {
			senc.EntryList[i].SubSamples[j].BytesOfClearData = binary.BigEndian.Uint16(buf[n:])
			n += 2
			senc.EntryList[i].SubSamples[j].BytesOfProtectedData = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
	}

	offset += n
	return
}
