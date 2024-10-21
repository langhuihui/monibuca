package box

import (
	"encoding/binary"
	"io"
	"time"

	"m7s.live/v5/pkg/util"
)

// aligned(8) class TrackHeaderBox
//    extends FullBox(‘tkhd’, version, flags){
//    if (version==1) {
//       unsigned int(64)  creation_time;
//       unsigned int(64)  modification_time;
//       unsigned int(32)  track_ID;
//       const unsigned int(32)  reserved = 0;
//       unsigned int(64)  duration;
//    } else { // version==0
//       unsigned int(32)  creation_time;
//       unsigned int(32)  modification_time;
//       unsigned int(32)  track_ID;
//       const unsigned int(32)  reserved = 0;
//       unsigned int(32)  duration;
// }
// const unsigned int(32)[2] reserved = 0;
// template int(16) layer = 0;
// template int(16) alternate_group = 0;
// template int(16) volume = {if track_is_audio 0x0100 else 0};
// const unsigned int(16) reserved = 0;
// template int(32)[9] matrix=
// { 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 };
//       // unity matrix
//    unsigned int(32) width;
//    unsigned int(32) height;
// }

type TrackHeaderBox struct {
	Creation_time     uint64
	Modification_time uint64
	Track_ID          uint32
	Duration          uint64
	Layer             uint16
	Alternate_group   uint16
	Volume            uint16
	Matrix            [9]uint32
	Width             uint32
	Height            uint32
}

func NewTrackHeaderBox() *TrackHeaderBox {
	_, offset := time.Now().Zone()
	return &TrackHeaderBox{
		Creation_time:     uint64(time.Now().Unix() + int64(offset) + 0x7C25B080),
		Modification_time: uint64(time.Now().Unix() + int64(offset) + 0x7C25B080),
		Layer:             0,
		Alternate_group:   0,
		Matrix:            [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	}
}

func (tkhd *TrackHeaderBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return 0, err
	}
	boxsize := 0
	if fullbox.Version == 0 {
		boxsize = 80
	} else {
		boxsize = 92
	}
	buf := make([]byte, boxsize)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	if fullbox.Version == 1 {
		tkhd.Creation_time = binary.BigEndian.Uint64(buf[n:])
		n += 8
		tkhd.Modification_time = binary.BigEndian.Uint64(buf[n:])
		n += 8
		tkhd.Track_ID = binary.BigEndian.Uint32(buf[n:])
		n += 8
		tkhd.Duration = binary.BigEndian.Uint64(buf[n:])
		n += 8
	} else {
		tkhd.Creation_time = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		tkhd.Modification_time = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		tkhd.Track_ID = binary.BigEndian.Uint32(buf[n:])
		n += 8
		tkhd.Duration = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	}
	n += 8
	tkhd.Layer = binary.BigEndian.Uint16(buf[n:])
	n += 2
	tkhd.Alternate_group = binary.BigEndian.Uint16(buf[n:])
	n += 2
	tkhd.Volume = binary.BigEndian.Uint16(buf[n:])
	n += 4
	for i := 0; i < 9; i++ {
		tkhd.Matrix[i] = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	tkhd.Width = binary.BigEndian.Uint32(buf[n:])
	tkhd.Height = binary.BigEndian.Uint32(buf[n+4:])
	offset += n + 8
	return
}

func (tkhd *TrackHeaderBox) Encode() (int, []byte) {
	fullbox := NewFullBox(TypeTKHD, util.Conditional[uint8](tkhd.Duration > 0xFFFFFFFF, 1, 0))
	fullbox.Flags[2] = 0x03 //Track_enabled | Track_in_movie
	fullbox.Box.Size = util.Conditional[uint64](fullbox.Version == 1, 92, 80) + FullBoxLen
	offset, buf := fullbox.Encode()
	if fullbox.Version == 1 {
		binary.BigEndian.PutUint64(buf[offset:], tkhd.Creation_time)
		offset += 8
		binary.BigEndian.PutUint64(buf[offset:], tkhd.Creation_time)
		offset += 8
		binary.BigEndian.PutUint32(buf[offset:], tkhd.Track_ID)
		offset += 8
		binary.BigEndian.PutUint64(buf[offset:], tkhd.Duration)
		offset += 8
	} else {
		binary.BigEndian.PutUint32(buf[offset:], uint32(tkhd.Creation_time))
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], uint32(tkhd.Creation_time))
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], tkhd.Track_ID)
		offset += 8
		binary.BigEndian.PutUint32(buf[offset:], uint32(tkhd.Duration))
		offset += 4
	}
	offset += 8
	binary.BigEndian.PutUint16(buf[offset:], tkhd.Layer)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], tkhd.Alternate_group)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], tkhd.Volume)
	offset += 4
	for i, _ := range tkhd.Matrix {
		binary.BigEndian.PutUint32(buf[offset:], tkhd.Matrix[i])
		offset += 4
	}
	binary.BigEndian.PutUint32(buf[offset:], uint32(tkhd.Width))
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], uint32(tkhd.Height))
	return offset + 4, buf
}
