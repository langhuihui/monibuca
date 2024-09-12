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

type SampleToChunkBox []STSCEntry

func NewSampleToChunkBox() *SampleToChunkBox {
	return &SampleToChunkBox{}
}

func (stsc SampleToChunkBox) Size() uint64 {
	return FullBoxLen + 4 + 12*uint64(len(stsc))
}

func (stsc *SampleToChunkBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 4)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	l := binary.BigEndian.Uint32(tmp)
	*stsc = make([]STSCEntry, l)
	buf := make([]byte, l*12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 8
	idx := 0
	for i := 0; i < int(l); i++ {
		entry := &(*stsc)[i]
		entry.FirstChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		entry.SamplesPerChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		entry.SampleDescriptionIndex = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	offset += idx
	return
}

func (stsc SampleToChunkBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeSTSC, 0)
	fullbox.Box.Size = stsc.Size()
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(stsc)))
	offset += 4
	for _, entry := range stsc {
		binary.BigEndian.PutUint32(buf[offset:], entry.FirstChunk)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], entry.SamplesPerChunk)
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], entry.SampleDescriptionIndex)
		offset += 4
	}
	return offset, buf
}
