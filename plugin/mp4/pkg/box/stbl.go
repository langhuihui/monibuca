package box

import (
	"bytes"
	"io"
)

type SampleTable struct {
	STSD *STSDBox
	STTS *STTSBox
	CTTS *CTTSBox
	STSC *STSCBox
	STSZ *STSZBox
	STSS *STSSBox
	STCO *STCOBox
}

type SampleTableBox struct {
	BaseBox
	SampleTable
}

func (stbl *SampleTableBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, stbl.STSD, stbl.STTS, stbl.CTTS, stbl.STSC, stbl.STSZ, stbl.STSS, stbl.STCO)
}

func (stbl *SampleTableBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)

	for {
		box, err := ReadFrom(r)

		if err != nil {
			break
		}
		switch box := box.(type) {
		case *STSDBox:
			stbl.STSD = box

		case *STTSBox:
			stbl.STTS = box
		case *CTTSBox:

			stbl.CTTS = box
		case *STCOBox:
			stbl.STCO = box
		case *CO64Box:
			co64 := STCOBox(*box)
			stbl.STCO = &co64
		case *STSCBox:
			stbl.STSC = box

		case *STSZBox:
			stbl.STSZ = box
		case *STSSBox:
			stbl.STSS = box
		}
	}
	return stbl, nil
}

func init() {
	RegisterBox[*SampleTableBox](TypeSTBL)
}
