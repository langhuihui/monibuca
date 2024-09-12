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

type TimeToSampleBox []STTSEntry

func (stts TimeToSampleBox) Size() uint64 {
	return FullBoxLen + 4 + 8*uint64(len(stts))
}

func (stts *TimeToSampleBox) Decode(r io.Reader) (offset int, err error) {
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
	*stts = make([]STTSEntry, l)
	buf := make([]byte, l*8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(l); i++ {
		(*stts)[i].SampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		(*stts)[i].SampleDelta = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (stts TimeToSampleBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeSTTS, 0)
	fullbox.Box.Size = stts.Size()
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(stts)))
	offset += 4
	for _, entry := range stts {
		binary.BigEndian.PutUint32(buf[offset:], entry.SampleCount)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], entry.SampleDelta)
		offset += 4
	}
	return offset, buf
}
