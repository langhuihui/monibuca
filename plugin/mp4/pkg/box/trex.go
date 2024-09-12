package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackExtendsBox extends FullBox(‘trex’, 0, 0){
// 	unsigned int(32) track_ID;
// 	unsigned int(32) default_sample_description_index;
// 	unsigned int(32) default_sample_duration;
// 	unsigned int(32) default_sample_size;
// 	unsigned int(32) default_sample_flags
// }

type TrackExtendsBox struct {
	Box                           *FullBox
	TrackID                       uint32
	DefaultSampleDescriptionIndex uint32
	DefaultSampleDuration         uint32
	DefaultSampleSize             uint32
	DefaultSampleFlags            uint32
}

func NewTrackExtendsBox(track uint32) *TrackExtendsBox {
	return &TrackExtendsBox{
		Box:     NewFullBox(TypeTREX, 0),
		TrackID: track,
	}
}

func (trex *TrackExtendsBox) Size() uint64 {
	return trex.Box.Size() + 20
}

func (trex *TrackExtendsBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = trex.Box.Decode(r); err != nil {
		return 0, err
	}

	buf := make([]byte, 20)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	trex.TrackID = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trex.DefaultSampleDescriptionIndex = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trex.DefaultSampleDuration = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trex.DefaultSampleSize = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trex.DefaultSampleFlags = binary.BigEndian.Uint32(buf[n:])
	n += 4
	return offset + 20, nil
}

func (trex *TrackExtendsBox) Encode() (int, []byte) {
	trex.Box.Box.Size = trex.Size()
	offset, buf := trex.Box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], trex.TrackID)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], trex.DefaultSampleDescriptionIndex)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], trex.DefaultSampleDuration)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], trex.DefaultSampleSize)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], trex.DefaultSampleFlags)
	offset += 4
	return offset, buf
}
