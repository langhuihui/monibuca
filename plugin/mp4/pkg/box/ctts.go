package box

import (
	"encoding/binary"
	"io"
)

type CTTSBox struct {
	FullBox
	Entries []CTTSEntry
}

func CreateCTTSBox(entries []CTTSEntry) *CTTSBox {
	return &CTTSBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeCTTS,
				size: uint32(FullBoxLen + 4 + len(entries)*8),
			},
		},
		Entries: entries,
	}
}

func (box *CTTSBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4+len(box.Entries)*8)
	// Write entry count
	binary.BigEndian.PutUint32(buf[:4], uint32(len(box.Entries)))

	// Write entries
	for i, entry := range box.Entries {
		binary.BigEndian.PutUint32(buf[4+i*8:], entry.SampleCount)
		binary.BigEndian.PutUint32(buf[4+i*8+4:], entry.SampleOffset)
	}

	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *CTTSBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]CTTSEntry, entryCount)

	if len(buf) < 4+int(entryCount)*8 {
		return nil, io.ErrShortBuffer
	}

	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i].SampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		box.Entries[i].SampleOffset = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	return box, nil
}

func init() {
	RegisterBox[CTTSBox](TypeCTTS)
}
