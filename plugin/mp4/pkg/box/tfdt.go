package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class TrackFragmentBaseMediaDecodeTimeBox extends FullBox('tfdt', version, 0) {
// 	if (version==1) {
// 		  unsigned int(64) baseMediaDecodeTime;
// 	   } else { // version==0
// 		  unsigned int(32) baseMediaDecodeTime;
// 	   }
// 	}

type TrackFragmentBaseMediaDecodeTimeBox struct {
	FullBox
	BaseMediaDecodeTime uint64
}

func CreateTrackFragmentBaseMediaDecodeTimeBox(baseMediaDecodeTime uint64) *TrackFragmentBaseMediaDecodeTimeBox {
	version := uint8(0)
	size := uint32(FullBoxLen + 4)
	if baseMediaDecodeTime > 0xFFFFFFFF {
		version = 1
		size = uint32(FullBoxLen + 8)
	}
	return &TrackFragmentBaseMediaDecodeTimeBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeTFDT,
				size: size,
			},
			Version: version,
			Flags:   [3]byte{0, 0, 0},
		},
		BaseMediaDecodeTime: baseMediaDecodeTime,
	}
}

func (box *TrackFragmentBaseMediaDecodeTimeBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	if box.Version == 1 {
		binary.BigEndian.PutUint64(tmp[:], box.BaseMediaDecodeTime)
		nn, err := w.Write(tmp[:8])
		return int64(nn), err
	} else {
		binary.BigEndian.PutUint32(tmp[:], uint32(box.BaseMediaDecodeTime))
		nn, err := w.Write(tmp[:4])
		return int64(nn), err
	}
}

func (box *TrackFragmentBaseMediaDecodeTimeBox) Unmarshal(buf []byte) (IBox, error) {
	if box.Version == 1 {
		if len(buf) < 8 {
			return nil, io.ErrShortBuffer
		}
		box.BaseMediaDecodeTime = binary.BigEndian.Uint64(buf[:8])
	} else {
		if len(buf) < 4 {
			return nil, io.ErrShortBuffer
		}
		box.BaseMediaDecodeTime = uint64(binary.BigEndian.Uint32(buf[:4]))
	}
	return box, nil
}

func init() {
	RegisterBox[*TrackFragmentBaseMediaDecodeTimeBox](TypeTFDT)
}
