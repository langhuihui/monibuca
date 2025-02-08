package box

import (
	"bytes"
	"io"
)

type MdiaBox struct {
	BaseBox
	MDHD *MediaHeaderBox
	MINF *MediaInformationBox
	HDLR *HandlerBox
}

func (m *MdiaBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, m.MDHD, m.MINF, m.HDLR)
}

func (m *MdiaBox) Unmarshal(buf []byte) (IBox, error) {
	for {
		b, err := ReadFrom(bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		switch box := b.(type) {
		case *MediaHeaderBox:
			m.MDHD = box
		case *MediaInformationBox:
			m.MINF = box
		case *HandlerBox:
			m.HDLR = box
		}
	}
}

type MediaInformationBox struct {
	BaseBox
	VMHD *VideoMediaHeaderBox
	SMHD *SoundMediaHeaderBox
	HMHD *HintMediaHeaderBox
	STBL *SampleTableBox
	DINF *DataInformationBox
}

func (m *MediaInformationBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, m.VMHD, m.SMHD, m.HMHD, m.STBL, m.DINF)
}

func (m *MediaInformationBox) Unmarshal(buf []byte) (IBox, error) {
	for {
		b, err := ReadFrom(bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		switch box := b.(type) {
		case *VideoMediaHeaderBox:
			m.VMHD = box
		case *SoundMediaHeaderBox:
			m.SMHD = box
		case *HintMediaHeaderBox:
			m.HMHD = box
		case *SampleTableBox:
			m.STBL = box
		case *DataInformationBox:
			m.DINF = box
		}
	}
}

func init() {
	RegisterBox[*MdiaBox](TypeMDIA)
}
