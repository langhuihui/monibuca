package box

import (
	"errors"
	"fmt"
	"io"
)

type Fmp4WriterSeeker struct {
	Buffer []byte
	Offset int
}

func NewFmp4WriterSeeker(capacity int) *Fmp4WriterSeeker {
	return &Fmp4WriterSeeker{
		Buffer: make([]byte, 0, capacity),
		Offset: 0,
	}
}

func (fws *Fmp4WriterSeeker) Write(p []byte) (n int, err error) {
	if cap(fws.Buffer)-fws.Offset >= len(p) {
		if len(fws.Buffer) < fws.Offset+len(p) {
			fws.Buffer = fws.Buffer[:fws.Offset+len(p)]
		}
		copy(fws.Buffer[fws.Offset:], p)
		fws.Offset += len(p)
		return len(p), nil
	}
	tmp := make([]byte, len(fws.Buffer), cap(fws.Buffer)+len(p)*2)
	copy(tmp, fws.Buffer)
	if len(fws.Buffer) < fws.Offset+len(p) {
		tmp = tmp[:fws.Offset+len(p)]
	}
	copy(tmp[fws.Offset:], p)
	fws.Buffer = tmp
	fws.Offset += len(p)
	return len(p), nil
}

func (fws *Fmp4WriterSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		if fws.Offset+int(offset) > len(fws.Buffer) {
			return -1, errors.New(fmt.Sprint("SeekCurrent out of range", len(fws.Buffer), offset, fws.Offset))
		}
		fws.Offset += int(offset)
		return int64(fws.Offset), nil
	} else if whence == io.SeekStart {
		if offset > int64(len(fws.Buffer)) {
			return -1, errors.New(fmt.Sprint("SeekStart out of range", len(fws.Buffer), offset, fws.Offset))
		}
		fws.Offset = int(offset)
		return offset, nil
	} else {
		return 0, errors.New("unsupport SeekEnd")
	}
}
