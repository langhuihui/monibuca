package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SyncSampleBox extends FullBox(‘stss’, version = 0, 0) {
//  	unsigned int(32) entry_count;
//  	int i;
//  	for (i=0; i < entry_count; i++) {
//  		unsigned int(32) sample_number;
//  	}
//  }

type SyncSampleBox []uint32

func (stss SyncSampleBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeSTSS, 0)
	fullbox.Box.Size = FullBoxLen + 4 + 4*uint64(len(stss))
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(stss)))
	offset += 4
	for _, sampleNumber := range stss {
		binary.BigEndian.PutUint32(buf[offset:], sampleNumber)
		offset += 4
	}
	return offset, buf
}

func (stss *SyncSampleBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 4)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	offset = 8
	entryCount := binary.BigEndian.Uint32(tmp[:])
	buf := make([]byte, entryCount*4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for range entryCount {
		*stss = append(*stss, binary.BigEndian.Uint32(buf[idx:]))
		idx += 4
	}
	offset += idx
	return
}
