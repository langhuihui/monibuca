package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SampleToChunkBox extends FullBox(‘stsc’, version = 0, 0) {
//     unsigned int(32) entry_count;
//     for (i=1; i <= entry_count; i++) {
//         unsigned int(32) first_chunk;
//         unsigned int(32) samples_per_chunk;
//         unsigned int(32) sample_description_index;
//     }
// }

type SampleToChunkBox struct {
	box        *FullBox
	stscentrys *movstsc
}

func NewSampleToChunkBox() *SampleToChunkBox {
	return &SampleToChunkBox{
		box: NewFullBox([4]byte{'s', 't', 's', 'c'}, 0),
	}
}

func (stsc *SampleToChunkBox) Size() uint64 {
	if stsc.stscentrys == nil {
		return stsc.box.Size()
	} else {
		return stsc.box.Size() + 4 + 12*uint64(stsc.stscentrys.entryCount)
	}
}

func (stsc *SampleToChunkBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = stsc.box.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 4)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	stsc.stscentrys = new(movstsc)
	stsc.stscentrys.entryCount = binary.BigEndian.Uint32(tmp)
	stsc.stscentrys.entrys = make([]stscEntry, stsc.stscentrys.entryCount)
	buf := make([]byte, stsc.stscentrys.entryCount*12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 8
	idx := 0
	for i := 0; i < int(stsc.stscentrys.entryCount); i++ {
		stsc.stscentrys.entrys[i].firstChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		stsc.stscentrys.entrys[i].samplesPerChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		stsc.stscentrys.entrys[i].sampleDescriptionIndex = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (stsc *SampleToChunkBox) Encode() (int, []byte) {
	stsc.box.Box.Size = stsc.Size()
	offset, buf := stsc.box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], stsc.stscentrys.entryCount)
	offset += 4
	for i := 0; i < int(stsc.stscentrys.entryCount); i++ {
		binary.BigEndian.PutUint32(buf[offset:], stsc.stscentrys.entrys[i].firstChunk)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], stsc.stscentrys.entrys[i].samplesPerChunk)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], stsc.stscentrys.entrys[i].sampleDescriptionIndex)
		offset += 4
	}
	return offset, buf
}

func makeStsc(stsc *movstsc) (boxdata []byte) {
	stscbox := NewSampleToChunkBox()
	stscbox.stscentrys = stsc
	_, boxdata = stscbox.Encode()
	return
}

func decodeStscBox(demuxer *MovDemuxer) (err error) {
	stsc := SampleToChunkBox{box: new(FullBox)}
	if _, err = stsc.Decode(demuxer.reader); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.stbltable.stsc = stsc.stscentrys
	return
}
