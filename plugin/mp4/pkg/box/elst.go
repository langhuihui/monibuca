package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class EditListBox extends FullBox('elst', version, 0) {
// 	unsigned int(32) entry_count;
// 	for (i=1; i <= entry_count; i++) {
// 		  if (version==1) {
// 			 unsigned int(64) segment_duration;
// 			 int(64) media_time;
// 		  } else { // version==0
// 			 unsigned int(32) segment_duration;
// 			 int(32)  media_time;
// 		  }
// 		  int(16) media_rate_integer;
// 		  int(16) media_rate_fraction = 0;
// 	}
// }

type EditListBox struct {
	FullBox
	Entries []ELSTEntry
}

func CreateEditListBox(version byte, entries []ELSTEntry) *EditListBox {
	entrySize := 12 // version 0: 4 + 4 + 2 + 2
	if version == 1 {
		entrySize = 20 // version 1: 8 + 8 + 2 + 2
	}
	return &EditListBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeELST,
				size: uint32(FullBoxLen + 4 + len(entries)*entrySize),
			},
			Version: version,
			Flags:   [3]byte{0, 0, 0},
		},
		Entries: entries,
	}
}

func (box *EditListBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	binary.BigEndian.PutUint32(tmp[:4], uint32(len(box.Entries)))
	_, err = w.Write(tmp[:4])
	if err != nil {
		return
	}
	n = 4

	for _, entry := range box.Entries {
		if box.Version == 1 {
			binary.BigEndian.PutUint64(tmp[:], entry.SegmentDuration)
			_, err = w.Write(tmp[:8])
			if err != nil {
				return
			}
			n += 8
			binary.BigEndian.PutUint64(tmp[:], uint64(entry.MediaTime))
			_, err = w.Write(tmp[:8])
			if err != nil {
				return
			}
			n += 8
		} else {
			binary.BigEndian.PutUint32(tmp[:], uint32(entry.SegmentDuration))
			_, err = w.Write(tmp[:4])
			if err != nil {
				return
			}
			n += 4
			binary.BigEndian.PutUint32(tmp[:], uint32(entry.MediaTime))
			_, err = w.Write(tmp[:4])
			if err != nil {
				return
			}
			n += 4
		}
		binary.BigEndian.PutUint16(tmp[:], uint16(entry.MediaRateInteger))
		_, err = w.Write(tmp[:2])
		if err != nil {
			return
		}
		n += 2
		binary.BigEndian.PutUint16(tmp[:], uint16(entry.MediaRateFraction))
		_, err = w.Write(tmp[:2])
		if err != nil {
			return
		}
		n += 2
	}
	return
}

func (box *EditListBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]ELSTEntry, entryCount)

	offset := 4
	for i := range box.Entries {
		if box.Version == 1 {
			if offset+20 > len(buf) {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].SegmentDuration = binary.BigEndian.Uint64(buf[offset:])
			offset += 8
			box.Entries[i].MediaTime = int64(binary.BigEndian.Uint64(buf[offset:]))
			offset += 8
		} else {
			if offset+12 > len(buf) {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].SegmentDuration = uint64(binary.BigEndian.Uint32(buf[offset:]))
			offset += 4
			box.Entries[i].MediaTime = int64(int32(binary.BigEndian.Uint32(buf[offset:])))
			offset += 4
		}
		box.Entries[i].MediaRateInteger = int16(binary.BigEndian.Uint16(buf[offset:]))
		offset += 2
		box.Entries[i].MediaRateFraction = int16(binary.BigEndian.Uint16(buf[offset:]))
		offset += 2
	}
	return box, nil
}

func init() {
	RegisterBox[*EditListBox](TypeELST)
}
