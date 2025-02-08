package box

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/yapingcat/gomedia/go-codec"
	"m7s.live/v5/pkg/util"
)

// aligned(8) class MediaHeaderBox extends FullBox('mdhd', version, 0) {
//  if (version==1) {
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
// bit(1) pad = 0;
// unsigned int(5)[3] language; // ISO-639-2/T language code
// unsigned int(16) pre_defined = 0;
// }

func ff_mov_iso639_to_lang(lang [3]byte) (code int) {
	for i := 0; i < 3; i++ {
		c := lang[i]
		c -= 0x60
		if c > 0x1f {
			return -1
		}
		code <<= 5
		code |= int(c)
	}
	return
}

type MediaHeaderBox struct {
	FullBox
	CreationTime     uint64
	ModificationTime uint64
	Timescale        uint32
	Duration         uint64
	Language         [3]byte
}

func CreateMediaHeaderBox(timescale uint32, duration uint64) *MediaHeaderBox {
	_, offset := time.Now().Zone()
	now := uint64(time.Now().Unix() + int64(offset) + 0x7C25B080)
	version := util.Conditional[uint8](duration > 0xFFFFFFFF, 1, 0)

	return &MediaHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeMDHD,
				size: util.Conditional[uint32](version == 1, 32, 20) + FullBoxLen,
			},
			Version: version,
			Flags:   [3]byte{0, 0, 0},
		},
		CreationTime:     now,
		ModificationTime: now,
		Timescale:        timescale,
		Duration:         duration,
		Language:         [3]byte{'u', 'n', 'd'},
	}
}

func (box *MediaHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	var data []byte
	if box.Version == 1 {
		data = make([]byte, 32)
		binary.BigEndian.PutUint64(data[0:], box.CreationTime)
		binary.BigEndian.PutUint64(data[8:], box.ModificationTime)
		binary.BigEndian.PutUint32(data[16:], box.Timescale)
		binary.BigEndian.PutUint64(data[20:], box.Duration)
		binary.BigEndian.PutUint16(data[28:], uint16(ff_mov_iso639_to_lang(box.Language)&0x7FFF))
		// pre_defined already zeroed
	} else {
		data = make([]byte, 20)
		binary.BigEndian.PutUint32(data[0:], uint32(box.CreationTime))
		binary.BigEndian.PutUint32(data[4:], uint32(box.ModificationTime))
		binary.BigEndian.PutUint32(data[8:], box.Timescale)
		binary.BigEndian.PutUint32(data[12:], uint32(box.Duration))
		binary.BigEndian.PutUint16(data[16:], uint16(ff_mov_iso639_to_lang(box.Language)&0x7FFF))
		// pre_defined already zeroed
	}

	nn, err := w.Write(data)
	n = int64(nn)
	return
}

func (box *MediaHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	if box.Version == 1 {
		if len(buf) < 32 {
			return nil, io.ErrShortBuffer
		}
		box.CreationTime = binary.BigEndian.Uint64(buf[0:])
		box.ModificationTime = binary.BigEndian.Uint64(buf[8:])
		box.Timescale = binary.BigEndian.Uint32(buf[16:])
		box.Duration = binary.BigEndian.Uint64(buf[20:])
		buf = buf[28:]
	} else {
		if len(buf) < 20 {
			return nil, io.ErrShortBuffer
		}
		box.CreationTime = uint64(binary.BigEndian.Uint32(buf[0:]))
		box.ModificationTime = uint64(binary.BigEndian.Uint32(buf[4:]))
		box.Timescale = binary.BigEndian.Uint32(buf[8:])
		box.Duration = uint64(binary.BigEndian.Uint32(buf[12:]))
		buf = buf[16:]
	}

	// Read language
	bs := codec.NewBitStream(buf)
	_ = bs.GetBit() // pad
	box.Language[0] = bs.Uint8(5)
	box.Language[1] = bs.Uint8(5)
	box.Language[2] = bs.Uint8(5)

	return box, nil
}

func init() {
	RegisterBox[*MediaHeaderBox](TypeMDHD)
}
