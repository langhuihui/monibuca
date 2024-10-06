package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SampleSizeBox extends FullBox(‘stsz’, version = 0, 0) {
// 		unsigned int(32) sample_size;
// 		unsigned int(32) sample_count;
// 		if (sample_size==0) {
// 		for (i=1; i <= sample_count; i++) {
// 		unsigned int(32) entry_size;
// 		}
// 	}
// }

type SampleSizeBox struct {
	SampleSize    uint32
	SampleCount   uint32
	EntrySizelist []uint32
}

func (stsz *SampleSizeBox) Size() uint64 {
	if stsz.SampleSize == 0 {
		return FullBoxLen + 8 + 4*uint64(stsz.SampleCount)
	} else {
		return FullBoxLen + 8
	}
}

func (stsz *SampleSizeBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 8)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	offset = 12
	stsz.SampleSize = binary.BigEndian.Uint32(tmp[:])
	stsz.SampleCount = binary.BigEndian.Uint32(tmp[4:])
	if stsz.SampleSize == 0 {
		buf := make([]byte, stsz.SampleCount*4)
		if _, err = io.ReadFull(r, buf); err != nil {
			return
		}
		idx := 0
		stsz.EntrySizelist = make([]uint32, stsz.SampleCount)
		for i := 0; i < int(stsz.SampleCount); i++ {
			stsz.EntrySizelist[i] = binary.BigEndian.Uint32(buf[idx:])
			idx += 4
		}
		offset += idx
	}
	return
}

func (stsz *SampleSizeBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeSTSZ, 0)
	fullbox.Box.Size = stsz.Size()
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], stsz.SampleSize)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], stsz.SampleCount)
	offset += 4
	if stsz.SampleSize == 0 {
		for i := 0; i < int(stsz.SampleCount); i++ {
			binary.BigEndian.PutUint32(buf[offset:], stsz.EntrySizelist[i])
			offset += 4
		}
	}
	return offset, buf
}
