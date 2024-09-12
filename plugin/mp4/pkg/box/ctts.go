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

type CompositionOffsetBox []CTTSEntry

func (ctts CompositionOffsetBox) Size() uint64 {
	return FullBoxLen + 4 + 8*uint64(len(ctts))
}

func (ctts *CompositionOffsetBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	entryCountBuf := make([]byte, 4)
	if _, err = io.ReadFull(r, entryCountBuf); err != nil {
		return
	}
	offset = 8
	l := binary.BigEndian.Uint32(entryCountBuf)
	*ctts = make([]CTTSEntry, l)

	buf := make([]byte, l*8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(l); i++ {
		(*ctts)[i].SampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		(*ctts)[i].SampleOffset = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (ctts CompositionOffsetBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeCTTS, 0)
	fullbox.Box.Size = ctts.Size()
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(ctts)))
	offset += 4
	for _, entry := range ctts {
		binary.BigEndian.PutUint32(buf[offset:], entry.SampleCount)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], entry.SampleOffset)
		offset += 4
	}
	return offset, buf
}
