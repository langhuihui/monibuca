package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SampleToChunkBox extends FullBox('stsc', version = 0, 0) {
//     unsigned int(32) entry_count;
//     for (i=1; i <= entry_count; i++) {
//         unsigned int(32) first_chunk;
//         unsigned int(32) samples_per_chunk;
//         unsigned int(32) sample_description_index;
//     }
// }

type STSCBox struct {
	FullBox
	Entries []STSCEntry
}

func CreateSTSCBox(entries []STSCEntry) *STSCBox {
	return &STSCBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSTSC,
				size: uint32(FullBoxLen + 4 + len(entries)*12),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		Entries: entries,
	}
}

func (box *STSCBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4+len(box.Entries)*12)
	// Write entry count
	binary.BigEndian.PutUint32(buf[:4], uint32(len(box.Entries)))

	// Write entries
	for i, entry := range box.Entries {
		binary.BigEndian.PutUint32(buf[4+i*12:], entry.FirstChunk)
		binary.BigEndian.PutUint32(buf[4+i*12+4:], entry.SamplesPerChunk)
		binary.BigEndian.PutUint32(buf[4+i*12+8:], entry.SampleDescriptionIndex)
	}

	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *STSCBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]STSCEntry, entryCount)

	if len(buf) < 4+int(entryCount)*12 {
		return nil, io.ErrShortBuffer
	}

	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i].FirstChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		box.Entries[i].SamplesPerChunk = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		box.Entries[i].SampleDescriptionIndex = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	return box, nil
}

func init() {
	RegisterBox[*STSCBox](TypeSTSC)
}
