package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackExtendsBox extends FullBox('trex', 0, 0){
// 	unsigned int(32) track_ID;
// 	unsigned int(32) default_sample_description_index;
// 	unsigned int(32) default_sample_duration;
// 	unsigned int(32) default_sample_size;
// 	unsigned int(32) default_sample_flags
// }

type TrackExtendsBox struct {
	FullBox
	TrackID                       uint32
	DefaultSampleDescriptionIndex uint32
	DefaultSampleDuration         uint32
	DefaultSampleSize             uint32
	DefaultSampleFlags            uint32
}

func CreateTrackExtendsBox(trackID uint32) *TrackExtendsBox {
	return &TrackExtendsBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTREX,
				size: uint32(FullBoxLen + 20),
			},
		},
		TrackID: trackID,
	}
}

func (box *TrackExtendsBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [20]byte
	binary.BigEndian.PutUint32(tmp[0:], box.TrackID)
	binary.BigEndian.PutUint32(tmp[4:], box.DefaultSampleDescriptionIndex)
	binary.BigEndian.PutUint32(tmp[8:], box.DefaultSampleDuration)
	binary.BigEndian.PutUint32(tmp[12:], box.DefaultSampleSize)
	binary.BigEndian.PutUint32(tmp[16:], box.DefaultSampleFlags)

	nn, err := w.Write(tmp[:])
	n = int64(nn)
	return
}

func (box *TrackExtendsBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 20 {
		return nil, io.ErrShortBuffer
	}

	box.TrackID = binary.BigEndian.Uint32(buf[0:])
	box.DefaultSampleDescriptionIndex = binary.BigEndian.Uint32(buf[4:])
	box.DefaultSampleDuration = binary.BigEndian.Uint32(buf[8:])
	box.DefaultSampleSize = binary.BigEndian.Uint32(buf[12:])
	box.DefaultSampleFlags = binary.BigEndian.Uint32(buf[16:])

	return box, nil
}

func init() {
	RegisterBox[*TrackExtendsBox](TypeTREX)
}
