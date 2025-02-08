package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SyncSampleBox extends FullBox('stss', version = 0, 0) {
//  	unsigned int(32) entry_count;
//  	int i;
//  	for (i=0; i < entry_count; i++) {
//  		unsigned int(32) sample_number;
//  	}
//  }

type STSSBox struct {
	FullBox
	Entries []uint32
}

func CreateSTSSBox(entries []uint32) *STSSBox {
	return &STSSBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSTSS,
				size: uint32(FullBoxLen + 4 + len(entries)*4),
			},
		},
		Entries: entries,
	}
}

func (box *STSSBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4*(len(box.Entries)+1))
	// Write entry count
	binary.BigEndian.PutUint32(buf[:4], uint32(len(box.Entries)))

	// Write entries
	for i, sampleNumber := range box.Entries {
		binary.BigEndian.PutUint32(buf[4+i*4:], sampleNumber)
	}
	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *STSSBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]uint32, entryCount)
	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i] = binary.BigEndian.Uint32(buf[idx:])
		idx += 4
	}
	return box, nil
}

func init() {
	RegisterBox[*STSSBox](TypeSTSS)
}
