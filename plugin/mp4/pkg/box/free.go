package box

import (
	"io"
)

type FreeBox struct {
	Data []byte
}

func (free *FreeBox) Decode(r io.Reader, baseBox *BasicBox) (int, error) {
	if BasicBoxLen < baseBox.Size {
		free.Data = make([]byte, baseBox.Size-BasicBoxLen)
		if _, err := io.ReadFull(r, free.Data); err != nil {
			return 0, err
		}
	}
	return int(baseBox.Size - BasicBoxLen), nil
}

func (free *FreeBox) Encode() []byte {
	offset, buf := (&BasicBox{Type: TypeFREE, Size: 8 + uint64(len(free.Data))}).Encode()
	copy(buf[offset:], free.Data)
	return buf
}
