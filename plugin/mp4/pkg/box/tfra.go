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
	NumberOfEntry         uint32
	FragEntrys            *movtfra
}

func NewTrackFragmentRandomAccessBox(trackid uint32) *TrackFragmentRandomAccessBox {
	return &TrackFragmentRandomAccessBox{
		Box:     NewFullBox([4]byte{'t', 'f', 'r', 'a'}, 1),
		TrackID: trackid,
	}
}

func (tfra *TrackFragmentRandomAccessBox) Size() uint64 {
	return tfra.Box.Size() + 12 + uint64(tfra.NumberOfEntry)*19
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
	tfra.NumberOfEntry = binary.BigEndian.Uint32(buf[n:])
	n += 4
	tfra.FragEntrys = new(movtfra)
	tfra.FragEntrys.frags = make([]fragEntry, tfra.NumberOfEntry)
	for i := 0; i < int(tfra.NumberOfEntry); i++ {
		if tfra.Box.Version == 1 {
			tfra.FragEntrys.frags[i].time = binary.BigEndian.Uint64(buf[n:])
			n += 8
			tfra.FragEntrys.frags[i].moofOffset = binary.BigEndian.Uint64(buf[n:])
			n += 8
		} else {
			tfra.FragEntrys.frags[i].time = uint64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
			tfra.FragEntrys.frags[i].moofOffset = uint64(binary.BigEndian.Uint32(buf[n:]))
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
	binary.BigEndian.PutUint32(boxdata[offset:], tfra.NumberOfEntry)
	offset += 4
	for i := 0; i < int(tfra.NumberOfEntry); i++ {
		if tfra.Box.Version == 1 {
			binary.BigEndian.PutUint64(boxdata[offset:], tfra.FragEntrys.frags[i].time)
			offset += 8
			binary.BigEndian.PutUint64(boxdata[offset:], tfra.FragEntrys.frags[i].moofOffset)
			offset += 8
		} else {
			binary.BigEndian.PutUint32(boxdata[offset:], uint32(tfra.FragEntrys.frags[i].time))
			offset += 4
			binary.BigEndian.PutUint32(boxdata[offset:], uint32(tfra.FragEntrys.frags[i].moofOffset))
			offset += 4
		}
		boxdata[offset] = 1
		boxdata[offset+1] = 1
		boxdata[offset+2] = 1
		offset += 3
	}
	return offset, boxdata
}

func makeTfraBox(track *mp4track) []byte {
	tfra := NewTrackFragmentRandomAccessBox(track.trackId)
	tfra.LengthSizeOfSampleNum = 0
	tfra.LengthSizeOfTrafNum = 0
	tfra.LengthSizeOfTrunNum = 0
	tfra.NumberOfEntry = uint32(len(track.fragments))
	frags := make([]fragEntry, 0, len(track.fragments))
	for i := 0; i < int(tfra.NumberOfEntry); i++ {
		frags = append(frags, fragEntry{
			time:       track.fragments[i].firstPts,
			moofOffset: track.fragments[i].offset,
		})
	}
	entrys := &movtfra{
		frags: frags,
	}
	tfra.FragEntrys = entrys
	_, tfraData := tfra.Encode()
	return tfraData
}
