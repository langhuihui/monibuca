package box

import (
	"errors"
	"fmt"
	"io"
)

type fmp4WriterSeeker struct {
	buffer []byte
	offset int
}

func newFmp4WriterSeeker(capacity int) *fmp4WriterSeeker {
	return &fmp4WriterSeeker{
		buffer: make([]byte, 0, capacity),
		offset: 0,
	}
}

func (fws *fmp4WriterSeeker) Write(p []byte) (n int, err error) {
	if cap(fws.buffer)-fws.offset >= len(p) {
		if len(fws.buffer) < fws.offset+len(p) {
			fws.buffer = fws.buffer[:fws.offset+len(p)]
		}
		copy(fws.buffer[fws.offset:], p)
		fws.offset += len(p)
		return len(p), nil
	}
	tmp := make([]byte, len(fws.buffer), cap(fws.buffer)+len(p)*2)
	copy(tmp, fws.buffer)
	if len(fws.buffer) < fws.offset+len(p) {
		tmp = tmp[:fws.offset+len(p)]
	}
	copy(tmp[fws.offset:], p)
	fws.buffer = tmp
	fws.offset += len(p)
	return len(p), nil
}

func (fws *fmp4WriterSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		if fws.offset+int(offset) > len(fws.buffer) {
			return -1, errors.New(fmt.Sprint("SeekCurrent out of range", len(fws.buffer), offset, fws.offset))
		}
		fws.offset += int(offset)
		return int64(fws.offset), nil
	} else if whence == io.SeekStart {
		if offset > int64(len(fws.buffer)) {
			return -1, errors.New(fmt.Sprint("SeekStart out of range", len(fws.buffer), offset, fws.offset))
		}
		fws.offset = int(offset)
		return offset, nil
	} else {
		return 0, errors.New("unsupport SeekEnd")
	}
}
