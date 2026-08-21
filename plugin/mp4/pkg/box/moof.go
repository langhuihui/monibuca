package box

import (
	"bytes"
	"io"

	"m7s.live/v5/pkg/collections"
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
			size: uint32(BasicBoxLen + tfhd.size + trun.size + tfdt.size),
		},
		TFHD: tfhd,
		TFDT: tfdt,
		TRUN: trun,
	}
}

func (box *MovieFragmentBox) WriteTo(w io.Writer) (n int64, err error) {
	boxes := append([]IBox{box.MFHD}, collections.Map(box.TRAFs, func(t *TrackFragmentBox) IBox { return t })...)
	return WriteTo(w, boxes...)
}

func (box *TrackFragmentBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, box.TFHD, box.TFDT, box.TRUN)
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
		case *TrackFragmentBaseMediaDecodeTimeBox:
			// Confirmed via 寸止: REQ-MP4-001 M2 — traf 内必须解析 tfdt，否则 fragment 时间基丢失
			box.TFDT = b
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
