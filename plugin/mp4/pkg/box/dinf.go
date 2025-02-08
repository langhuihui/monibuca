package box

import (
	"bytes"
	"encoding/binary"
	"io"
)

var (
	TypeURL = BoxType{'u', 'r', 'l', ' '}
	TypeURN = BoxType{'u', 'r', 'n', ' '}
)

// aligned(8) class DataEntryUrlBox (bit(24) flags) extends FullBox('url ', version = 0, flags) {
// 	string location;
// }
// aligned(8) class DataEntryUrnBox (bit(24) flags) extends FullBox('urn ', version = 0, flags) {
// 	string name;
// 	string location;
// }
// aligned(8) class DataReferenceBox extends FullBox('dref', version = 0, 0) {
// 	unsigned int(32)  entry_count;
//     for (i=1; i <= entry_count; i++) {
// 		DataEntryBox(entry_version, entry_flags) data_entry;
// 	}
// }

type DataInformationBox struct {
	BaseBox
	Dref *DataReferenceBox
}

type DataReferenceBox struct {
	FullBox
	Entries []IBox
}

type DataEntryUrlBox struct {
	FullBox
	Location string
}

type DataEntryUrnBox struct {
	FullBox
	Name     string
	Location string
}

func CreateDataInformationBox() *DataInformationBox {
	dref := CreateDataReferenceBox()
	return &DataInformationBox{
		BaseBox: BaseBox{
			typ:  TypeDINF,
			size: uint32(BasicBoxLen + dref.size),
		},
		Dref: dref,
	}
}

func CreateDataReferenceBox() *DataReferenceBox {
	url := CreateDataEntryUrlBox("")
	return &DataReferenceBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeDREF,
				size: uint32(FullBoxLen + 4 + url.size), // 4 for entry_count
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		Entries: []IBox{url},
	}
}

func CreateDataEntryUrlBox(location string) *DataEntryUrlBox {
	return &DataEntryUrlBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeURL,
				size: uint32(FullBoxLen + len(location)),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 1}, // self-contained flag
		},
		Location: location,
	}
}

func CreateDataEntryUrnBox(name, location string) *DataEntryUrnBox {
	return &DataEntryUrnBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeURN,
				size: uint32(FullBoxLen + len(name) + 1 + len(location)),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		Name:     name,
		Location: location,
	}
}

func (box *DataInformationBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, box.Dref)
}

func (box *DataInformationBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)
	b, err := ReadFrom(r)
	if err != nil {
		return nil, err
	}
	if dref, ok := b.(*DataReferenceBox); ok {
		box.Dref = dref
	}
	return box, nil
}

func (box *DataReferenceBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(len(box.Entries)))
	nn, err := w.Write(tmp[:])
	if err != nil {
		return int64(nn), err
	}
	n = int64(nn)

	for _, entry := range box.Entries {
		var en int64
		en, err = WriteTo(w, entry)
		if err != nil {
			return
		}
		n += en
	}
	return
}

func (box *DataReferenceBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 4 {
		return nil, io.ErrShortBuffer
	}
	entryCount := binary.BigEndian.Uint32(buf)
	r := bytes.NewReader(buf[4:])
	box.Entries = make([]IBox, 0, entryCount)

	for i := uint32(0); i < entryCount; i++ {
		entry, err := ReadFrom(r)
		if err != nil {
			break
		}
		box.Entries = append(box.Entries, entry)
	}
	return box, nil
}

func (box *DataEntryUrlBox) WriteTo(w io.Writer) (n int64, err error) {
	if len(box.Location) > 0 {
		nn, err := w.Write([]byte(box.Location))
		return int64(nn), err
	}
	return 0, nil
}

func (box *DataEntryUrlBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) > 0 {
		box.Location = string(buf)
	}
	return box, nil
}

func (box *DataEntryUrnBox) WriteTo(w io.Writer) (n int64, err error) {
	nn, err := w.Write([]byte(box.Name + "\x00" + box.Location))
	return int64(nn), err
}

func (box *DataEntryUrnBox) Unmarshal(buf []byte) (IBox, error) {
	parts := bytes.SplitN(buf, []byte{0}, 2)
	if len(parts) > 0 {
		box.Name = string(parts[0])
		if len(parts) > 1 {
			box.Location = string(parts[1])
		}
	}
	return box, nil
}

func init() {
	RegisterBox[*DataInformationBox](TypeDINF)
	RegisterBox[*DataReferenceBox](TypeDREF)
	RegisterBox[*DataEntryUrlBox](TypeURL)
	RegisterBox[*DataEntryUrnBox](TypeURN)
}
