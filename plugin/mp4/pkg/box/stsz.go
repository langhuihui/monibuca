package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class SampleSizeBox extends FullBox('stsz', version = 0, 0) {
// 		unsigned int(32) sample_size;
// 		unsigned int(32) sample_count;
// 		if (sample_size==0) {
// 		for (i=1; i <= sample_count; i++) {
// 		unsigned int(32) entry_size;
// 		}
// 	}
// }

type STSZBox struct {
	FullBox
	SampleSize    uint32
	SampleCount   uint32
	EntrySizelist []uint32
}

func CreateSTSZBox(sampleSize uint32, entrySizelist []uint32) *STSZBox {
	return &STSZBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSTSZ,
				size: uint32(FullBoxLen + 8 + len(entrySizelist)*4),
			},
		},
		SampleSize:    sampleSize,
		SampleCount:   uint32(len(entrySizelist)),
		EntrySizelist: entrySizelist,
	}
}

func (box *STSZBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 8+len(box.EntrySizelist)*4)

	// Write sample size
	binary.BigEndian.PutUint32(buf[:4], box.SampleSize)

	// Write sample count
	binary.BigEndian.PutUint32(buf[4:8], box.SampleCount)

	// Write entry sizes if sample size is 0
	if box.SampleSize == 0 {
		for i, size := range box.EntrySizelist {
			binary.BigEndian.PutUint32(buf[8+i*4:], size)
		}
	}

	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *STSZBox) Unmarshal(buf []byte) (IBox, error) {
	box.SampleSize = binary.BigEndian.Uint32(buf[:4])
	box.SampleCount = binary.BigEndian.Uint32(buf[4:8])

	if box.SampleSize == 0 {
		if len(buf) < 8+int(box.SampleCount)*4 {
			return nil, io.ErrShortBuffer
		}
		box.EntrySizelist = make([]uint32, box.SampleCount)
		idx := 8
		for i := 0; i < int(box.SampleCount); i++ {
			box.EntrySizelist[i] = binary.BigEndian.Uint32(buf[idx:])
			idx += 4
		}
	}

	return box, nil
}

func init() {
	RegisterBox[*STSZBox](TypeSTSZ)
}
