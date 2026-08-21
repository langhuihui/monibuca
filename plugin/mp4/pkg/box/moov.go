package box

import (
	"bytes"
	"io"

	"m7s.live/v5/pkg/collections"
)

type (
	MoovBox struct {
		BaseBox
		Tracks []*TrakBox
		UDTA   *UserDataBox
		MVHD   *MovieHeaderBox
		MVEX   *MovieExtendsBox
	}

	EdtsBox struct {
		BaseBox
		Elst *EditListBox
	}
)

func (m *MoovBox) WriteTo(w io.Writer) (n int64, err error) {
	boxes := append([]IBox{m.MVHD}, collections.Map(m.Tracks, func(t *TrakBox) IBox { return t })...)
	if m.MVEX != nil {
		boxes = append(boxes, m.MVEX)
	}
	if m.UDTA != nil {
		boxes = append(boxes, m.UDTA)
	}
	return WriteTo(w, boxes...)
}

func (m *MoovBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)
	for {
		b, err := ReadFrom(r)
		if err != nil {
			return m, err
		}
		switch box := b.(type) {
		case *TrakBox:
			m.Tracks = append(m.Tracks, box)
		case *MovieHeaderBox:
			m.MVHD = box
		case *MovieExtendsBox:
			m.MVEX = box
		case *UserDataBox:
			m.UDTA = box
		}
	}
}

func (e *EdtsBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, e.Elst)
}

func (e *EdtsBox) Unmarshal(buf []byte) (b IBox, err error) {
	r := bytes.NewReader(buf)
	for err == nil {
		b, err = ReadFrom(r)
		if err != nil {
			return e, err
		}
		switch box := b.(type) {
		case *EditListBox:
			e.Elst = box
		}
	}
	return e, err
}

func init() {
	RegisterBox[*MoovBox](TypeMOOV)
	RegisterBox[*EdtsBox](TypeEDTS)
}
