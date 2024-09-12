package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackFragmentBaseMediaDecodeTimeBox extends FullBox(‘tfdt’, version, 0) {
// 	if (version==1) {
// 		  unsigned int(64) baseMediaDecodeTime;
// 	   } else { // version==0
// 		  unsigned int(32) baseMediaDecodeTime;
// 	   }
// 	}

type TrackFragmentBaseMediaDecodeTimeBox struct {
	BaseMediaDecodeTime uint64
}

func NewTrackFragmentBaseMediaDecodeTimeBox(fragStart uint64) *TrackFragmentBaseMediaDecodeTimeBox {
	return &TrackFragmentBaseMediaDecodeTimeBox{
		BaseMediaDecodeTime: fragStart,
	}
}

func (tfdt *TrackFragmentBaseMediaDecodeTimeBox) Size() uint64 {
	return FullBoxLen + 8
}

func (tfdt *TrackFragmentBaseMediaDecodeTimeBox) Decode(r io.Reader, size uint32) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}

	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	if fullbox.Version == 1 {
		tfdt.BaseMediaDecodeTime = binary.BigEndian.Uint64(buf)
		offset += 8
	} else {
		tfdt.BaseMediaDecodeTime = uint64(binary.BigEndian.Uint32(buf))
		offset += 4
	}
	return
}

func (tfdt *TrackFragmentBaseMediaDecodeTimeBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeTFDT, 1)
	fullbox.Box.Size = tfdt.Size()
	offset, boxdata := fullbox.Encode()
	binary.BigEndian.PutUint64(boxdata[offset:], tfdt.BaseMediaDecodeTime)
	return offset + 8, boxdata
}
