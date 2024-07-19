package box

import (
	"encoding/binary"
	"io"
	"time"
)

// aligned(8) class MovieHeaderBox extends FullBox(‘mvhd’, version, 0) {
// if (version==1) {
// 	unsigned int(64)  creation_time;
// 	unsigned int(64)  modification_time;
// 	unsigned int(32)  timescale;
// 	unsigned int(64)  duration;
//  } else { // version==0
// 	unsigned int(32)  creation_time;
// 	unsigned int(32)  modification_time;
// 	unsigned int(32)  timescale;
// 	unsigned int(32)  duration;
// }
// template int(32) rate = 0x00010000; // typically 1.0
// template int(16) volume = 0x0100; // typically, full volume const bit(16) reserved = 0;
// const unsigned int(32)[2] reserved = 0;
// template int(32)[9] matrix =
// { 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 };
// 	// Unity matrix
//  bit(32)[6]  pre_defined = 0;
//  unsigned int(32)  next_track_ID;
// }

type MovieHeaderBox struct {
	Box               *FullBox
	Creation_time     uint64
	Modification_time uint64
	Timescale         uint32
	Duration          uint64
	Rate              uint32
	Volume            uint16
	Matrix            [9]uint32
	Pre_defined       [6]uint32
	Next_track_ID     uint32
}

func NewMovieHeaderBox() *MovieHeaderBox {
	_, offset := time.Now().Zone()
	return &MovieHeaderBox{
		Box:               NewFullBox([4]byte{'m', 'v', 'h', 'd'}, 0),
		Creation_time:     uint64(time.Now().Unix() + int64(offset) + 0x7C25B080),
		Modification_time: uint64(time.Now().Unix() + int64(offset) + 0x7C25B080),
		Timescale:         1000,
		Rate:              0x00010000,
		Volume:            0x0100,
		Matrix:            [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	}
}

func (mvhd *MovieHeaderBox) Size() uint64 {
	if mvhd.Box.Version == 1 {
		return mvhd.Box.Size() + 108
	} else {
		return mvhd.Box.Size() + 96
	}
}

func (mvhd *MovieHeaderBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = mvhd.Box.Decode(r); err != nil {
		return 0, err
	}
	boxsize := 0
	if mvhd.Box.Version == 0 {
		boxsize = 96
	} else {
		boxsize = 108
	}
	buf := make([]byte, boxsize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	if mvhd.Box.Version == 1 {
		mvhd.Creation_time = binary.BigEndian.Uint64(buf[n:])
		n += 8
		mvhd.Modification_time = binary.BigEndian.Uint64(buf[n:])
		n += 8
		mvhd.Timescale = binary.BigEndian.Uint32(buf[n:])
		n += 4
		mvhd.Duration = binary.BigEndian.Uint64(buf[n:])
		n += 8
	} else {
		mvhd.Creation_time = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		mvhd.Modification_time = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
		mvhd.Timescale = binary.BigEndian.Uint32(buf[n:])
		n += 4
		mvhd.Duration = uint64(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	}
	mvhd.Rate = binary.BigEndian.Uint32(buf[n:])
	n += 4
	mvhd.Volume = binary.BigEndian.Uint16(buf[n:])
	n += 10

	for i, _ := range mvhd.Matrix {
		mvhd.Matrix[i] = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}

	for i := 0; i < 6; i++ {
		mvhd.Pre_defined[i] = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	mvhd.Next_track_ID = binary.BigEndian.Uint32(buf[n:])
	return n + 4 + offset, nil
}

func (mvhd *MovieHeaderBox) Encode() (int, []byte) {
	mvhd.Box.Box.Size = mvhd.Size()
	offset, buf := mvhd.Box.Encode()
	if mvhd.Box.Version == 1 {
		binary.BigEndian.PutUint64(buf[offset:], mvhd.Creation_time)
		offset += 8
		binary.BigEndian.PutUint64(buf[offset:], mvhd.Modification_time)
		offset += 8
		binary.BigEndian.PutUint32(buf[offset:], mvhd.Timescale)
		offset += 4
		binary.BigEndian.PutUint64(buf[offset:], mvhd.Duration)
		offset += 8
	} else {
		binary.BigEndian.PutUint32(buf[offset:], uint32(mvhd.Creation_time))
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], uint32(mvhd.Modification_time))
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], uint32(mvhd.Timescale))
		offset += 4
		binary.BigEndian.PutUint32(buf[offset:], uint32(mvhd.Duration))
		offset += 4
	}
	binary.BigEndian.PutUint32(buf[offset:], mvhd.Rate)
	offset += 4
	binary.BigEndian.PutUint16(buf[offset:], mvhd.Volume)
	offset += 2
	offset += 10
	for i, _ := range mvhd.Matrix {
		binary.BigEndian.PutUint32(buf[offset:], mvhd.Matrix[i])
		offset += 4
	}
	offset += 24
	binary.BigEndian.PutUint32(buf[offset:], mvhd.Next_track_ID)
	return offset + 2, buf
}

func makeMvhdBox(trackid uint32, duration uint32) []byte {
	mvhd := NewMovieHeaderBox()
	mvhd.Next_track_ID = trackid
	mvhd.Duration = uint64(duration)
	_, mvhdbox := mvhd.Encode()
	return mvhdbox
}

func decodeMvhd(demuxer *MovDemuxer) (err error) {
	mvhd := MovieHeaderBox{Box: new(FullBox)}
	if _, err = mvhd.Decode(demuxer.reader); err != nil {
		return
	}
	demuxer.mp4Info.Duration = uint32(mvhd.Duration)
	demuxer.mp4Info.Timescale = mvhd.Timescale
	demuxer.mp4Info.CreateTime = mvhd.Creation_time
	demuxer.mp4Info.ModifyTime = mvhd.Modification_time
	return
}
