package box

import (
	"bytes"
	"io"
)

// aligned(8) class MovieExtendsBox extends Box('mvex') {
// }

type MovieExtendsBox struct {
	BaseBox
	Trexs []*TrackExtendsBox
}

func CreateMovieExtendsBox(trexs []*TrackExtendsBox) *MovieExtendsBox {
	size := uint32(BasicBoxLen)
	for _, trex := range trexs {
		size += trex.size
	}

	return &MovieExtendsBox{
		BaseBox: BaseBox{
			typ:  TypeMVEX,
			size: size,
		},
		Trexs: trexs,
	}
}

func (box *MovieExtendsBox) WriteTo(w io.Writer) (n int64, err error) {
	boxes := make([]IBox, len(box.Trexs))
	for i, trex := range box.Trexs {
		boxes[i] = trex
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
		if trex, ok := b.(*TrackExtendsBox); ok {
			box.Trexs = append(box.Trexs, trex)
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*MovieExtendsBox](TypeMVEX)
}
