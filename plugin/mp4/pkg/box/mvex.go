package box

import (
	"bytes"
	"encoding/binary"
	"io"
)

// aligned(8) class MovieExtendsBox extends Box('mvex') {
// }

type MovieExtendsHeaderBox struct {
	FullBox
	FragmentDuration uint32
}

func CreateMovieExtendsHeaderBox(fragmentDuration uint32) *MovieExtendsHeaderBox {
	return &MovieExtendsHeaderBox{
		FullBox: FullBox{
			BaseBox: BaseBox{typ: TypeMEHD, size: FullBoxLen + 4},
		},
		FragmentDuration: fragmentDuration,
	}
}

func (box *MovieExtendsHeaderBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, box.FragmentDuration)
	w.Write(buf)
	return int64(len(buf)), nil
}

func (box *MovieExtendsHeaderBox) Unmarshal(buf []byte) (IBox, error) {
	box.FragmentDuration = binary.BigEndian.Uint32(buf)
	return box, nil
}

type MovieExtendsBox struct {
	BaseBox
	Mehd  *MovieExtendsHeaderBox
	Trexs []*TrackExtendsBox
}

func CreateMovieExtendsBox(mehd *MovieExtendsHeaderBox, trexs []*TrackExtendsBox) *MovieExtendsBox {
	size := uint32(BasicBoxLen)
	if mehd != nil {
		size += mehd.size
	}
	for _, trex := range trexs {
		size += trex.size
	}

	return &MovieExtendsBox{
		BaseBox: BaseBox{
			typ:  TypeMVEX,
			size: size,
		},
		Mehd:  mehd,
		Trexs: trexs,
	}
}

func (box *MovieExtendsBox) WriteTo(w io.Writer) (n int64, err error) {
	boxes := make([]IBox, len(box.Trexs)+1)
	boxes[0] = box.Mehd
	for i, trex := range box.Trexs {
		boxes[i+1] = trex
	}
	return WriteTo(w, boxes...)
}

func (box *MovieExtendsBox) Unmarshal(buf []byte) (IBox, error) {
	box.Trexs = make([]*TrackExtendsBox, 0)
	r := bytes.NewReader(buf)
	for {
		b, err := ReadFrom(r)
		if err != nil {
			break
		}
		switch v := b.(type) {
		case *MovieExtendsHeaderBox:
			box.Mehd = v
		case *TrackExtendsBox:
			box.Trexs = append(box.Trexs, v)
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*MovieExtendsBox](TypeMVEX)
	RegisterBox[*MovieExtendsHeaderBox](TypeMEHD)
}
