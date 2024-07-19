package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class CompositionOffsetBox extends FullBox(‘ctts’, version = 0, 0) {
//     unsigned int(32) entry_count;
//     int i;
//     if (version==0) {
//         for (i=0; i < entry_count; i++) {
//             unsigned int(32) sample_count;
//             unsigned int(32) sample_offset;
//         }
//     }
//     else if (version == 1) {
//         for (i=0; i < entry_count; i++) {
//             unsigned int(32) sample_count;
//             signed int(32) sample_offset;
//         }
//     }
// }

type CompositionOffsetBox struct {
	box  *FullBox
	ctts *movctts
}

func NewCompositionOffsetBox() *CompositionOffsetBox {
	return &CompositionOffsetBox{
		box: NewFullBox([4]byte{'c', 't', 't', 's'}, 0),
	}
}
func (ctts *CompositionOffsetBox) Size() uint64 {
	if ctts.ctts == nil {
		return ctts.box.Size()
	} else {
		return ctts.box.Size() + 4 + 8*uint64(ctts.ctts.entryCount)
	}
}

func (ctts *CompositionOffsetBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = ctts.box.Decode(r); err != nil {
		return
	}
	entryCountBuf := make([]byte, 4)
	if _, err = io.ReadFull(r, entryCountBuf); err != nil {
		return
	}
	offset = 8
	ctts.ctts = new(movctts)
	ctts.ctts.entryCount = binary.BigEndian.Uint32(entryCountBuf)
	ctts.ctts.entrys = make([]cttsEntry, ctts.ctts.entryCount)

	buf := make([]byte, ctts.ctts.entryCount*8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(ctts.ctts.entryCount); i++ {
		ctts.ctts.entrys[i].sampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		ctts.ctts.entrys[i].sampleOffset = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (ctts *CompositionOffsetBox) Encode() (int, []byte) {
	ctts.box.Box.Size = ctts.Size()
	offset, buf := ctts.box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], ctts.ctts.entryCount)
	offset += 4
	for i := 0; i < int(ctts.ctts.entryCount); i++ {
		binary.BigEndian.PutUint32(buf[offset:], ctts.ctts.entrys[i].sampleCount)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], ctts.ctts.entrys[i].sampleOffset)
		offset += 4
	}
	return offset, buf
}

func makeCtts(ctts *movctts) (boxdata []byte) {
	cttsbox := NewCompositionOffsetBox()
	cttsbox.ctts = ctts
	_, boxdata = cttsbox.Encode()
	return
}

func decodeCttsBox(demuxer *MovDemuxer) (err error) {
	ctts := CompositionOffsetBox{box: new(FullBox)}
	if _, err = ctts.Decode(demuxer.reader); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.stbltable.ctts = ctts.ctts
	return
}
