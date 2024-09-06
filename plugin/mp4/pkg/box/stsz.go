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
	box  *FullBox
	stsz *movstsz
}

func NewSampleSizeBox() *SampleSizeBox {
	return &SampleSizeBox{
		box: NewFullBox([4]byte{'s', 't', 's', 'z'}, 0),
	}
}

func (stsz *SampleSizeBox) Size() uint64 {
	if stsz.stsz == nil {
		return stsz.box.Size()
	} else if stsz.stsz.sampleSize == 0 {
		return stsz.box.Size() + 8 + 4*uint64(stsz.stsz.sampleCount)
	} else {
		return stsz.box.Size() + 8
	}
}

func (stsz *SampleSizeBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = stsz.box.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 8)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	offset = 12
	stsz.stsz = new(movstsz)
	stsz.stsz.sampleSize = binary.BigEndian.Uint32(tmp[:])
	stsz.stsz.sampleCount = binary.BigEndian.Uint32(tmp[4:])
	if stsz.stsz.sampleSize == 0 {
		buf := make([]byte, stsz.stsz.sampleCount*4)
		if _, err = io.ReadFull(r, buf); err != nil {
			return
		}
		idx := 0
		stsz.stsz.entrySizelist = make([]uint32, stsz.stsz.sampleCount)
		for i := 0; i < int(stsz.stsz.sampleCount); i++ {
			stsz.stsz.entrySizelist[i] = binary.BigEndian.Uint32(buf[idx:])
			idx += 4
		}
		offset += idx
	}
	return
}

func (stsz *SampleSizeBox) Encode() (int, []byte) {
	stsz.box.Box.Size = stsz.Size()
	offset, buf := stsz.box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], stsz.stsz.sampleSize)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], stsz.stsz.sampleCount)
	offset += 4
	if stsz.stsz.sampleSize == 0 {
		for i := 0; i < int(stsz.stsz.sampleCount); i++ {
			binary.BigEndian.PutUint32(buf[offset:], stsz.stsz.entrySizelist[i])
			offset += 4
		}
	}
	return offset, buf
}

func makeStsz(stsz *movstsz) (boxdata []byte) {
	stszbox := NewSampleSizeBox()
	stszbox.stsz = stsz
	_, boxdata = stszbox.Encode()
	return
}

func decodeStszBox(demuxer *MovDemuxer) (err error) {
	stsz := SampleSizeBox{box: new(FullBox)}
	if _, err = stsz.Decode(demuxer.reader); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.stbltable.stsz = stsz.stsz
	return
}
