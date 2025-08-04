package box

import (
	"encoding/binary"
	"io"
	"time"

	"m7s.live/v5/pkg/util"
)

// aligned(8) class TrackHeaderBox
//    extends FullBox('tkhd', version, flags){
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
	FullBox
	CreationTime     uint64
	ModificationTime uint64
	TrackID          uint32
	Duration         uint64
	Layer            uint16
	AlternateGroup   uint16
	Volume           uint16
	Matrix           [9]uint32
	Width            uint32
	Height           uint32
}

func CreateTrackHeaderBox(trackID uint32, duration uint64) *TrackHeaderBox {
	now := ConvertUnixTimeToISO14496(uint64(time.Now().Unix()))
	version := util.Conditional[uint8](duration > 0xFFFFFFFF, 1, 0)
	if duration == 0 {
		now = 0
	}
	return &TrackHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTKHD,
				size: util.Conditional[uint32](version == 1, 92, 80) + FullBoxLen,
			},
			Version: version,
			Flags:   [3]byte{0, 0, 7}, // Track_enabled | Track_in_movie | Track_in_preview
		},
		CreationTime:     now,
		ModificationTime: now,
		TrackID:          trackID,
		Duration:         duration,
		Layer:            0,
		AlternateGroup:   0,
		Matrix:           [9]uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000},
	}
}

func (box *TrackHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var data []byte

	if box.Version == 1 {
		data = make([]byte, 92)
		binary.BigEndian.PutUint64(data[0:], box.CreationTime)
		binary.BigEndian.PutUint64(data[8:], box.ModificationTime)
		binary.BigEndian.PutUint32(data[16:], box.TrackID)
		binary.BigEndian.PutUint64(data[24:], box.Duration)
		// 32-40 reserved
	} else {
		data = make([]byte, 80)
		binary.BigEndian.PutUint32(data[0:], uint32(box.CreationTime))
		binary.BigEndian.PutUint32(data[4:], uint32(box.ModificationTime))
		binary.BigEndian.PutUint32(data[8:], box.TrackID)
		binary.BigEndian.PutUint32(data[16:], uint32(box.Duration))
		// 20-28 reserved
	}

	offset := util.Conditional[int](box.Version == 1, 32, 20)
	// 8 bytes reserved already zeroed
	offset += 8
	binary.BigEndian.PutUint16(data[offset:], box.Layer)
	binary.BigEndian.PutUint16(data[offset+2:], box.AlternateGroup)
	binary.BigEndian.PutUint16(data[offset+4:], box.Volume)
	// 2 bytes reserved already zeroed
	offset += 8

	for i, m := range box.Matrix {
		binary.BigEndian.PutUint32(data[offset+i*4:], m)
	}
	offset += 36

	binary.BigEndian.PutUint32(data[offset:], box.Width)
	binary.BigEndian.PutUint32(data[offset+4:], box.Height)

	nn, err := w.Write(data)
	n = int64(nn)
	return
}

func (box *TrackHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if box.Version == 1 {
		if len(buf) < 92 {
			return nil, io.ErrShortBuffer
		}
		box.CreationTime = binary.BigEndian.Uint64(buf[0:])
		box.ModificationTime = binary.BigEndian.Uint64(buf[8:])
		box.TrackID = binary.BigEndian.Uint32(buf[16:])
		box.Duration = binary.BigEndian.Uint64(buf[24:])
		buf = buf[32:]
	} else {
		if len(buf) < 80 {
			return nil, io.ErrShortBuffer
		}
		box.CreationTime = uint64(binary.BigEndian.Uint32(buf[0:]))
		box.ModificationTime = uint64(binary.BigEndian.Uint32(buf[4:]))
		box.TrackID = binary.BigEndian.Uint32(buf[8:])
		box.Duration = uint64(binary.BigEndian.Uint32(buf[16:]))
		buf = buf[20:]
	}

	buf = buf[8:] // skip reserved
	box.Layer = binary.BigEndian.Uint16(buf[0:])
	box.AlternateGroup = binary.BigEndian.Uint16(buf[2:])
	box.Volume = binary.BigEndian.Uint16(buf[4:])
	buf = buf[8:] // skip reserved

	for i := 0; i < 9; i++ {
		box.Matrix[i] = binary.BigEndian.Uint32(buf[i*4:])
	}
	buf = buf[36:]

	box.Width = binary.BigEndian.Uint32(buf[0:])
	box.Height = binary.BigEndian.Uint32(buf[4:])

	return box, nil
}

func init() {
	RegisterBox[*TrackHeaderBox](TypeTKHD)
}
