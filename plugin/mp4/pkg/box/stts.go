package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TimeToSampleBox extends FullBox(’stts’, version = 0, 0) {
//     unsigned int(32) entry_count;
//     int i;
//     for (i=0; i < entry_count; i++) {
//         unsigned int(32) sample_count;
//         unsigned int(32) sample_delta;
//     }
// }

type TimeToSampleBox struct {
	box       *FullBox
	entryList *movstts
}

func NewTimeToSampleBox() *TimeToSampleBox {
	return &TimeToSampleBox{
		box: NewFullBox([4]byte{'s', 't', 't', 's'}, 0),
	}
}

func (stts *TimeToSampleBox) Size() uint64 {
	if stts.entryList == nil {
		return stts.box.Size()
	} else {
		return stts.box.Size() + 4 + 8*uint64(stts.entryList.entryCount)
	}
}

func (stts *TimeToSampleBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = stts.box.Decode(r); err != nil {
		return
	}
	entryCountBuf := make([]byte, 4)
	if _, err = io.ReadFull(r, entryCountBuf); err != nil {
		return
	}
	offset = 8
	stts.entryList = new(movstts)
	stts.entryList.entryCount = binary.BigEndian.Uint32(entryCountBuf)
	stts.entryList.entrys = make([]sttsEntry, stts.entryList.entryCount)
	buf := make([]byte, stts.entryList.entryCount*8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(stts.entryList.entryCount); i++ {
		stts.entryList.entrys[i].sampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		stts.entryList.entrys[i].sampleDelta = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (stts *TimeToSampleBox) Encode() (int, []byte) {
	stts.box.Box.Size = stts.Size()
	offset, buf := stts.box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], stts.entryList.entryCount)
	offset += 4
	for i := 0; i < int(stts.entryList.entryCount); i++ {
		binary.BigEndian.PutUint32(buf[offset:], stts.entryList.entrys[i].sampleCount)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], stts.entryList.entrys[i].sampleDelta)
		offset += 4
	}
	return offset, buf
}

func makeStts(stts *movstts) (boxdata []byte) {
	sttsbox := NewTimeToSampleBox()
	sttsbox.entryList = stts
	_, boxdata = sttsbox.Encode()
	return
}

func decodeSttsBox(demuxer *MovDemuxer) (err error) {
	stts := TimeToSampleBox{box: new(FullBox)}
	if _, err = stts.Decode(demuxer.reader); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.stbltable.stts = stts.entryList
	return
}
