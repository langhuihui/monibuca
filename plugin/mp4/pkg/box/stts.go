package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TimeToSampleBox extends FullBox('stts', version = 0, 0) {
//     unsigned int(32) entry_count;
//     int i;
//     for (i=0; i < entry_count; i++) {
//         unsigned int(32) sample_count;
//         unsigned int(32) sample_delta;
//     }
// }

type STTSBox struct {
	FullBox
	Entries []STTSEntry
}

func CreateSTTSBox(entries []STTSEntry) *STTSBox {
	return &STTSBox{
		FullBox: FullBox{
			BaseBox: BaseBox{

				typ:  TypeSTTS,
				size: uint32(FullBoxLen + 4 + len(entries)*8),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},

		Entries: entries,
	}
}

func (box *STTSBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4+len(box.Entries)*8)
	// Write entry count
	binary.BigEndian.PutUint32(buf[:4], uint32(len(box.Entries)))

	// Write entries
	for i, entry := range box.Entries {
		binary.BigEndian.PutUint32(buf[4+i*8:], entry.SampleCount)
		binary.BigEndian.PutUint32(buf[4+i*8+4:], entry.SampleDelta)
	}

	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *STTSBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]STTSEntry, entryCount)

	if len(buf) < 4+int(entryCount)*8 {
		return nil, io.ErrShortBuffer
	}

	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i].SampleCount = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
		box.Entries[i].SampleDelta = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	return box, nil
}

func init() {
	RegisterBox[*STTSBox](TypeSTTS)
}
