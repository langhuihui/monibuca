package box

import (
	"bytes"
	"io"
)

// aligned(8) class MovieFragmentBox extends Box('moof'){
// }

type MovieFragmentBox struct {
	BaseBox
	MFHD  *MovieFragmentHeaderBox
	TRAFs []*TrackFragmentBox
}

type TrackFragmentBox struct {
	BaseBox
	TFHD *TrackFragmentHeaderBox
	TFDT *TrackFragmentBaseMediaDecodeTimeBox
	TRUN *TrackRunBox
}

func CreateTrackFragmentBox(tfhd *TrackFragmentHeaderBox, tfdt *TrackFragmentBaseMediaDecodeTimeBox, trun *TrackRunBox) *TrackFragmentBox {
	return &TrackFragmentBox{
		BaseBox: BaseBox{
			typ:  TypeTRAF,
			size: uint32(BasicBoxLen + tfhd.size + trun.size),
		},
		TFHD: tfhd,
		TFDT: tfdt,
		TRUN: trun,
	}
}

func (box *MovieFragmentBox) WriteTo(w io.Writer) (n int64, err error) {
	boxes := []IBox{box.MFHD}
	for _, traf := range box.TRAFs {
		boxes = append(boxes, traf)
	}
	return WriteTo(w, boxes...)
}

func (box *TrackFragmentBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, box.TFHD, box.TRUN)
}

func (box *MovieFragmentBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)
	for {
		b, err := ReadFrom(r)
		if err != nil {
			break
		}
		switch b := b.(type) {
		case *MovieFragmentHeaderBox:
			box.MFHD = b
		case *TrackFragmentBox:
			box.TRAFs = append(box.TRAFs, b)
		}
	}
	return box, nil
}

func (box *TrackFragmentBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)
	for {
		b, err := ReadFrom(r)
		if err != nil {
			break
		}
		switch b := b.(type) {
		case *TrackFragmentHeaderBox:
			box.TFHD = b
		case *TrackRunBox:
			box.TRUN = b
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*MovieFragmentBox](TypeMOOF)
	RegisterBox[*TrackFragmentBox](TypeTRAF)
}
