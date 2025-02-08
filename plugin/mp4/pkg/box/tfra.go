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
	FullBox
	TrackID               uint32
	LengthSizeOfTrafNum   uint8
	LengthSizeOfTrunNum   uint8
	LengthSizeOfSampleNum uint8
	Entries               []TFRAEntry
}

func CreateTrackFragmentRandomAccessBox(trackID uint32, entries []TFRAEntry) *TrackFragmentRandomAccessBox {
	return &TrackFragmentRandomAccessBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTFRA,
				size: uint32(FullBoxLen + 8 + 4 + len(entries)*(3+8)),
			},
		},
		TrackID: trackID,
		Entries: entries,
	}
}

func (box *TrackFragmentRandomAccessBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [12]byte
	binary.BigEndian.PutUint32(tmp[:4], box.TrackID)
	tmp[7] = box.LengthSizeOfTrafNum<<6 | box.LengthSizeOfTrunNum<<4 | box.LengthSizeOfSampleNum<<2
	binary.BigEndian.PutUint32(tmp[8:], uint32(len(box.Entries)))
	nn, err := w.Write(tmp[:])
	if err != nil {
		return
	}
	n = int64(nn)

	for _, entry := range box.Entries {
		if box.Version == 1 {
			binary.BigEndian.PutUint64(tmp[:], entry.Time)
			nn, err = w.Write(tmp[:8])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)

			binary.BigEndian.PutUint64(tmp[:], entry.MoofOffset)
			nn, err = w.Write(tmp[:8])
			if err != nil {
				return n + int64(nn), err
			}
			n += int64(nn)
		} else {
			binary.BigEndian.PutUint32(tmp[:4], uint32(entry.Time))
			binary.BigEndian.PutUint32(tmp[4:8], uint32(entry.MoofOffset))
			nn, err = w.Write(tmp[:8])
			if err != nil {
				return
			}
			n += int64(nn)
		}

		trafSize := box.LengthSizeOfTrafNum + 1
		trunSize := box.LengthSizeOfTrunNum + 1
		sampleSize := box.LengthSizeOfSampleNum + 1

		switch trafSize {
		case 4:
			binary.BigEndian.PutUint32(tmp[:], entry.TrafNumber)
		case 3:
			binary.BigEndian.PutUint32(tmp[:], entry.TrafNumber&0x00FFFFFF)
		case 2:
			binary.BigEndian.PutUint16(tmp[:], uint16(entry.TrafNumber))
		case 1:
			tmp[0] = uint8(entry.TrafNumber)
		}
		switch trunSize {
		case 4:
			binary.BigEndian.PutUint32(tmp[trafSize:], entry.TrunNumber)
		case 3:
			binary.BigEndian.PutUint32(tmp[trafSize:], entry.TrunNumber&0x00FFFFFF)
		case 2:
			binary.BigEndian.PutUint16(tmp[trafSize:], uint16(entry.TrunNumber))
		case 1:
			tmp[trafSize] = uint8(entry.TrunNumber)
		}

		switch sampleSize {
		case 4:
			binary.BigEndian.PutUint32(tmp[trafSize+trunSize:], entry.SampleNumber)
		case 3:
			binary.BigEndian.PutUint32(tmp[trafSize+trunSize:], entry.SampleNumber&0x00FFFFFF)
		case 2:
			binary.BigEndian.PutUint16(tmp[trafSize+trunSize:], uint16(entry.SampleNumber))
		case 1:
			tmp[trafSize+trunSize] = uint8(entry.SampleNumber)
		}
		nn, err = w.Write(tmp[:trafSize+trunSize+sampleSize])
		if err != nil {
			return
		}
		n += int64(nn)
	}

	return
}

func (box *TrackFragmentRandomAccessBox) Unmarshal(buf []byte) (IBox, error) {

	box.TrackID = binary.BigEndian.Uint32(buf[:4])
	flags := buf[7]
	box.LengthSizeOfTrafNum = (flags >> 6) & 0x03
	box.LengthSizeOfTrunNum = (flags >> 4) & 0x03
	box.LengthSizeOfSampleNum = (flags >> 2) & 0x03
	entryCount := binary.BigEndian.Uint32(buf[8:])

	n := 12
	box.Entries = make([]TFRAEntry, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		if box.Version == 1 {
			if len(buf) < n+16 {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].Time = binary.BigEndian.Uint64(buf[n:])
			n += 8
			box.Entries[i].MoofOffset = binary.BigEndian.Uint64(buf[n:])
			n += 8
		} else {
			if len(buf) < n+8 {
				return nil, io.ErrShortBuffer
			}
			box.Entries[i].Time = uint64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
			box.Entries[i].MoofOffset = uint64(binary.BigEndian.Uint32(buf[n:]))
			n += 4
		}

		trafSize := box.LengthSizeOfTrafNum + 1
		trunSize := box.LengthSizeOfTrunNum + 1
		sampleSize := box.LengthSizeOfSampleNum + 1

		if len(buf) < n+int(trafSize+trunSize+sampleSize) {
			return nil, io.ErrShortBuffer
		}

		switch trafSize {
		case 4:
			box.Entries[i].TrafNumber = binary.BigEndian.Uint32(buf[n:])
		case 3:
			box.Entries[i].TrafNumber = binary.BigEndian.Uint32(buf[n:]) & 0x00FFFFFF
		case 2:
			box.Entries[i].TrafNumber = uint32(binary.BigEndian.Uint16(buf[n:]))
		case 1:
			box.Entries[i].TrafNumber = uint32(buf[n])
		}
		n += int(trafSize)

		switch trunSize {
		case 4:
			box.Entries[i].TrunNumber = binary.BigEndian.Uint32(buf[n:])
		case 3:
			box.Entries[i].TrunNumber = binary.BigEndian.Uint32(buf[n:]) & 0x00FFFFFF
		case 2:
			box.Entries[i].TrunNumber = uint32(binary.BigEndian.Uint16(buf[n:]))
		case 1:
			box.Entries[i].TrunNumber = uint32(buf[n])
		}
		n += int(trunSize)

		switch sampleSize {
		case 4:
			box.Entries[i].SampleNumber = binary.BigEndian.Uint32(buf[n:])
		case 3:
			box.Entries[i].SampleNumber = binary.BigEndian.Uint32(buf[n:]) & 0x00FFFFFF
		case 2:
			box.Entries[i].SampleNumber = uint32(binary.BigEndian.Uint16(buf[n:]))
		case 1:
			box.Entries[i].SampleNumber = uint32(buf[n])
		}
		n += int(sampleSize)
	}

	return box, nil
}

func init() {
	RegisterBox[*TrackFragmentRandomAccessBox](TypeTFRA)
}
