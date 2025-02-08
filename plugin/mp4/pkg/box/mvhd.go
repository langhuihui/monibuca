package box

import (
	"encoding/binary"
	"io"
	"time"
)

// aligned(8) class MovieHeaderBox extends FullBox('mvhd', version, 0) {
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
	FullBox
	CreationTime     uint64
	ModificationTime uint64
	Timescale        uint32
	Duration         uint64
	Rate             int32
	Volume           int16
	Matrix           [9]int32
	NextTrackID      uint32
}

func CreateMovieHeaderBox(nextTrackID uint32, duration uint32) *MovieHeaderBox {
	now := time.Now().Unix()
	return &MovieHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeMVHD,
				size: uint32(FullBoxLen + 96),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		CreationTime:     uint64(now),
		ModificationTime: uint64(now),
		Timescale:        1000,
		Duration:         uint64(duration),
		Rate:             0x00010000,
		Volume:           0x0100,
		Matrix:           [9]int32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
		NextTrackID:      nextTrackID,
	}
}

func (box *MovieHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var nn int
	if box.Version == 1 {
		var tmp [108]byte
		binary.BigEndian.PutUint64(tmp[0:], box.CreationTime)
		binary.BigEndian.PutUint64(tmp[8:], box.ModificationTime)
		binary.BigEndian.PutUint32(tmp[16:], box.Timescale)
		binary.BigEndian.PutUint64(tmp[20:], box.Duration)
		binary.BigEndian.PutUint32(tmp[28:], uint32(box.Rate))
		binary.BigEndian.PutUint16(tmp[32:], uint16(box.Volume))
		offset := 34 + 8
		for i := 0; i < 9; i++ {
			binary.BigEndian.PutUint32(tmp[offset:], uint32(box.Matrix[i]))
			offset += 4
		}
		binary.BigEndian.PutUint32(tmp[104:], box.NextTrackID)
		nn, err = w.Write(tmp[:])
		n = int64(nn)
	} else {
		var tmp [96]byte
		binary.BigEndian.PutUint32(tmp[0:], uint32(box.CreationTime))
		binary.BigEndian.PutUint32(tmp[4:], uint32(box.ModificationTime))
		binary.BigEndian.PutUint32(tmp[8:], box.Timescale)
		binary.BigEndian.PutUint32(tmp[12:], uint32(box.Duration))
		binary.BigEndian.PutUint32(tmp[16:], uint32(box.Rate))
		binary.BigEndian.PutUint16(tmp[20:], uint16(box.Volume))
		offset := 22 + 8
		for i := 0; i < 9; i++ {
			binary.BigEndian.PutUint32(tmp[offset:], uint32(box.Matrix[i]))
			offset += 4
		}
		binary.BigEndian.PutUint32(tmp[92:], box.NextTrackID)
		nn, err = w.Write(tmp[:])
		n = int64(nn)
	}
	return
}

func (box *MovieHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	var offset int
	if box.Version == 1 {
		box.CreationTime = binary.BigEndian.Uint64(buf[0:])
		box.ModificationTime = binary.BigEndian.Uint64(buf[8:])
		box.Timescale = binary.BigEndian.Uint32(buf[16:])
		box.Duration = binary.BigEndian.Uint64(buf[20:])
		offset = 28
	} else {
		box.CreationTime = uint64(binary.BigEndian.Uint32(buf[0:]))
		box.ModificationTime = uint64(binary.BigEndian.Uint32(buf[4:]))
		box.Timescale = binary.BigEndian.Uint32(buf[8:])
		box.Duration = uint64(binary.BigEndian.Uint32(buf[12:]))
		offset = 16
	}

	box.Rate = int32(binary.BigEndian.Uint32(buf[offset:]))
	box.Volume = int16(binary.BigEndian.Uint16(buf[offset+4:]))
	// skip reserved: 2 + 8 bytes

	offset += 16
	for i := 0; i < 9; i++ {
		box.Matrix[i] = int32(binary.BigEndian.Uint32(buf[offset:]))
		offset += 4
	}
	// skip pre_defined: 24 bytes
	box.NextTrackID = binary.BigEndian.Uint32(buf[offset+24:])

	return box, nil
}

func init() {
	RegisterBox[*MovieHeaderBox](TypeMVHD)
}
