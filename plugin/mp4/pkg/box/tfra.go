package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackFragmentRandomAccessBox
// extends FullBox(‘tfra’, version, 0) {
// 	unsigned int(32)  track_ID;
// 	const unsigned int(26)  reserved = 0;
// 	unsigned int(2) length_size_of_traf_num;
// 	unsigned int(2) length_size_of_trun_num;
// 	unsigned int(2)  length_size_of_sample_num;
// 	unsigned int(32)  number_of_entry;
// 	for(i=1; i <= number_of_entry; i++){
// 		if(version==1){
// 			unsigned int(64)  time;
// 			unsigned int(64)  moof_offset;
// 		 }else{
// 			unsigned int(32)  time;
// 			unsigned int(32)  moof_offset;
// 		 }
// 		 unsignedint((length_size_of_traf_num+1)*8) traf_number;
// 		 unsignedint((length_size_of_trun_num+1)*8) trun_number;
// 		 unsigned int((length_size_of_sample_num+1) * 8)sample_number;
// 	}
// }

type TrackFragmentRandomAccessBox struct {
	Box                   *FullBox
	TrackID               uint32
	LengthSizeOfTrafNum   uint8
	LengthSizeOfTrunNum   uint8
	LengthSizeOfSampleNum uint8
	FragEntrys            []FragEntry
}

func NewTrackFragmentRandomAccessBox(trackid uint32) *TrackFragmentRandomAccessBox {
	return &TrackFragmentRandomAccessBox{
		Box:     NewFullBox(TypeTFRA, 1),
		TrackID: trackid,
	}
}

func (tfra *TrackFragmentRandomAccessBox) Size() uint64 {
	return tfra.Box.Size() + 12 + uint64(len(tfra.FragEntrys))*19
}

func (tfra *TrackFragmentRandomAccessBox) Decode(r io.Reader) (offset int, err error) {
	if offset, err = tfra.Box.Decode(r); err != nil {
		return
	}

	needSize := tfra.Box.Box.Size - 12
	buf := make([]byte, needSize)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	tfra.TrackID = binary.BigEndian.Uint32(buf[n:])
	n += 4
	tfra.LengthSizeOfTrafNum = (buf[n+3] >> 4) & 0x03
	tfra.LengthSizeOfTrunNum = (buf[n+3] >> 2) & 0x03
	tfra.LengthSizeOfSampleNum = buf[n+3] & 0x03
	n += 4
	tfra.FragEntrys = make([]FragEntry, binary.BigEndian.Uint32(buf[n:]))
	n += 4
	for i := range tfra.FragEntrys {
		frag := &tfra.FragEntrys[i]
		if tfra.Box.Version == 1 {
			frag.Time = binary.BigEndian.Uint64(buf[n:])
			n += 8
			frag.MoofOffset = binary.BigEndian.Uint64(buf[n:])
			n += 8
		} else {
			frag.Time = uint64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
			frag.MoofOffset = uint64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
		}
		n += int(tfra.LengthSizeOfTrafNum + tfra.LengthSizeOfTrunNum + tfra.LengthSizeOfSampleNum + 3)
	}
	offset += 4
	return
}

func (tfra *TrackFragmentRandomAccessBox) Encode() (int, []byte) {
	tfra.Box.Box.Size = tfra.Size()
	offset, boxdata := tfra.Box.Encode()
	binary.BigEndian.PutUint32(boxdata[offset:], tfra.TrackID)
	offset += 4
	binary.BigEndian.PutUint32(boxdata[offset:], 0)
	offset += 4
	binary.BigEndian.PutUint32(boxdata[offset:], uint32(len(tfra.FragEntrys)))
	offset += 4
	for _, frag := range tfra.FragEntrys {
		if tfra.Box.Version == 1 {
			binary.BigEndian.PutUint64(boxdata[offset:], frag.Time)
			offset += 8
			binary.BigEndian.PutUint64(boxdata[offset:], frag.MoofOffset)
			offset += 8
		} else {
			binary.BigEndian.PutUint32(boxdata[offset:], uint32(frag.Time))
			offset += 4
			binary.BigEndian.PutUint32(boxdata[offset:], uint32(frag.MoofOffset))
			offset += 4
		}
		boxdata[offset] = 1
		boxdata[offset+1] = 1
		boxdata[offset+2] = 1
		offset += 3
	}
	return offset, boxdata
}
