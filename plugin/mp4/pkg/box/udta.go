package box

import (
	"bytes"
	"io"
)

// User Data Box (udta)
// This box contains objects that declare user information about the containing box and its data.
type UserDataBox struct {
	BaseBox
	Entries []IBox
}

// Custom metadata box for storing stream path
type StreamPathBox struct {
	FullBox
	StreamPath string
}

// Create a new User Data Box
func CreateUserDataBox(entries ...IBox) *UserDataBox {
	size := uint32(BasicBoxLen)
	for _, entry := range entries {
		size += uint32(entry.Size())
	}
	return &UserDataBox{
		BaseBox: BaseBox{
			typ:  TypeUDTA,
			size: size,
		},
		Entries: entries,
	}
}

// Create a new StreamPath Box
func CreateStreamPathBox(streamPath string) *StreamPathBox {
	return &StreamPathBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeM7SP, // Custom box type for M7S StreamPath
				size: uint32(FullBoxLen + len(streamPath)),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},
		StreamPath: streamPath,
	}
}

// WriteTo writes the UserDataBox to the given writer
func (box *UserDataBox) WriteTo(w io.Writer) (n int64, err error) {
	return WriteTo(w, box.Entries...)
}

// Unmarshal parses the given buffer into a UserDataBox
func (box *UserDataBox) Unmarshal(buf []byte) (IBox, error) {
	r := bytes.NewReader(buf)
	for {
		b, err := ReadFrom(r)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		box.Entries = append(box.Entries, b)
	}
	return box, nil
}

// WriteTo writes the StreamPathBox to the given writer
func (box *StreamPathBox) WriteTo(w io.Writer) (n int64, err error) {
	nn, err := w.Write([]byte(box.StreamPath))
	return int64(nn), err
}

// Unmarshal parses the given buffer into a StreamPathBox
func (box *StreamPathBox) Unmarshal(buf []byte) (IBox, error) {
	box.StreamPath = string(buf)
	return box, nil
}

func init() {
	RegisterBox[*UserDataBox](TypeUDTA)
	RegisterBox[*StreamPathBox](TypeM7SP)
}
