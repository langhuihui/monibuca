package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class EditListBox extends FullBox(‘elst’, version, 0) {
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
	Version byte
	Entrys  []ELSTEntry
}

func NewEditListBox(version byte) *EditListBox {
	return &EditListBox{
		Version: version,
	}
}

func (elst *EditListBox) Encode(boxSize int) (int, []byte) {
	fullbox := NewFullBox(TypeELST, elst.Version)
	fullbox.Box.Size = uint64(boxSize)
	offset, elstdata := fullbox.Encode()
	binary.BigEndian.PutUint32(elstdata[offset:], uint32(len(elst.Entrys)))
	offset += 4
	for _, entry := range elst.Entrys {
		if elst.Version == 1 {
			binary.BigEndian.PutUint64(elstdata[offset:], entry.SegmentDuration)
			offset += 8
			binary.BigEndian.PutUint64(elstdata[offset:], uint64(entry.MediaTime))
			offset += 8
		} else {
			binary.BigEndian.PutUint32(elstdata[offset:], uint32(entry.SegmentDuration))
			offset += 4
			binary.BigEndian.PutUint32(elstdata[offset:], uint32(entry.MediaTime))
			offset += 4
		}
		binary.BigEndian.PutUint16(elstdata[offset:], uint16(entry.MediaRateInteger))
		offset += 2
		binary.BigEndian.PutUint16(elstdata[offset:], uint16(entry.MediaRateFraction))
		offset += 2
	}
	return offset, elstdata
}

func (elst *EditListBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return 0, err
	}

	entryCountBuf := make([]byte, 4)
	if _, err = io.ReadFull(r, entryCountBuf); err != nil {
		return
	}
	entryCount := binary.BigEndian.Uint32(entryCountBuf)
	offset += 4
	var boxsize uint32
	if elst.Version == 0 {
		boxsize = 12 * entryCount
	} else {
		boxsize = 20 * entryCount
	}
	buf := make([]byte, boxsize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	if elst.Entrys == nil {
		elst.Entrys = make([]ELSTEntry, entryCount)
	}
	nn := 0
	for i := range entryCount {
		entry := &elst.Entrys[i]
		if elst.Version == 0 {
			entry.SegmentDuration = uint64(binary.BigEndian.Uint32(buf[nn:]))
			nn += 4
			entry.MediaTime = int64(int32(binary.BigEndian.Uint32(buf[nn:])))
			nn += 4
		} else {
			entry.SegmentDuration = uint64(binary.BigEndian.Uint64(buf[nn:]))
			nn += 8
			entry.MediaTime = int64(binary.BigEndian.Uint64(buf[nn:]))
			nn += 8
		}
		entry.MediaRateInteger = int16(binary.BigEndian.Uint16(buf[nn:]))
		nn += 2
		entry.MediaRateFraction = int16(binary.BigEndian.Uint16(buf[nn:]))
		nn += 2
	}
	return offset + nn, nil
}
