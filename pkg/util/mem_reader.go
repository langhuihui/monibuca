package util

import (
	"io"
	"slices"
)

type MemoryReader struct {
	*Memory
	Length, offset0, offset1 int
}

func NewReadableBuffersFromBytes(b ...[]byte) MemoryReader {
	buf := &Memory{Buffers: b}
	for _, level0 := range b {
		buf.Size += len(level0)
	}
	return MemoryReader{Memory: buf, Length: buf.Size}
}

var _ io.Reader = (*MemoryReader)(nil)

func (r *MemoryReader) Offset() int {
	return r.Size - r.Length
}

func (r *MemoryReader) Pop() []byte {
	panic("ReadableBuffers Pop not allowed")
}

func (r *MemoryReader) GetCurrent() []byte {
	return r.Memory.Buffers[r.offset0][r.offset1:]
}

func (r *MemoryReader) MoveToEnd() {
	r.offset0 = r.Count()
	r.offset1 = 0
	r.Length = 0
}

func (r *MemoryReader) Read(buf []byte) (actual int, err error) {
	if r.Length == 0 {
		return 0, io.EOF
	}
	n := len(buf)
	curBuf := r.GetCurrent()
	curBufLen := len(curBuf)
	if n > r.Length {
		if curBufLen > 0 {
			actual += copy(buf, curBuf)
			r.offset0++
			r.offset1 = 0
		}
		for _, b := range r.Memory.Buffers[r.offset0:] {
			actual += copy(buf[actual:], b)
		}
		r.MoveToEnd()
		return
	}
	l := n
	for n > 0 {
		curBuf = r.GetCurrent()
		curBufLen = len(curBuf)
		if n < curBufLen {
			actual += n
			copy(buf[l-n:], curBuf[:n])
			r.forward(n)
			break
		}
		copy(buf[l-n:], curBuf)
		n -= curBufLen
		actual += curBufLen
		r.skipBuf()
		if r.Length == 0 && n > 0 {
			err = io.EOF
			return
		}
	}
	return
}
func (r *MemoryReader) ReadByteTo(b ...*byte) (err error) {
	for i := range b {
		if r.Length == 0 {
			return io.EOF
		}
		*b[i], err = r.ReadByte()
		if err != nil {
			return
		}
	}
	return
}

func (r *MemoryReader) ReadByteMask(mask byte) (byte, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	return b & mask, nil
}

func (r *MemoryReader) ReadByte() (b byte, err error) {
	if r.Length == 0 {
		return 0, io.EOF
	}
	curBuf := r.GetCurrent()
	b = curBuf[0]
	if len(curBuf) == 1 {
		r.skipBuf()
	} else {
		r.forward(1)
	}
	return
}

func (r *MemoryReader) LEB128Unmarshal() (uint, int, error) {
	v := uint(0)
	n := 0
	for i := 0; i < 8; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, 0, err
		}
		v |= uint(b&0b01111111) << (i * 7)
		n++

		if (b & 0b10000000) == 0 {
			break
		}
	}

	return v, n, nil
}
func (r *MemoryReader) getCurrentBufLen() int {
	return len(r.Memory.Buffers[r.offset0]) - r.offset1
}
func (r *MemoryReader) Skip(n int) error {
	if n <= 0 {
		return nil
	}
	if n > r.Length {
		return io.EOF
	}
	curBufLen := r.getCurrentBufLen()
	for n > 0 {
		if n < curBufLen {
			r.forward(n)
			break
		}
		n -= curBufLen
		r.skipBuf()
		if r.Length == 0 && n > 0 {
			return io.EOF
		}
	}
	return nil
}

func (r *MemoryReader) Unread(n int) {
	r.Length += n
	r.offset1 -= n
	for r.offset1 < 0 {
		r.offset0--
		r.offset1 += len(r.Memory.Buffers[r.offset0])
	}
}

func (r *MemoryReader) forward(n int) {
	r.Length -= n
	r.offset1 += n
}

func (r *MemoryReader) skipBuf() {
	curBufLen := r.getCurrentBufLen()
	r.Length -= curBufLen
	r.offset0++
	r.offset1 = 0
}

func (r *MemoryReader) ReadBytes(n int) ([]byte, error) {
	if n > r.Length {
		return nil, io.EOF
	}
	b := make([]byte, n)
	actual, err := r.Read(b)
	return b[:actual], err
}

func (r *MemoryReader) ReadBE(n int) (num uint32, err error) {
	for i := range n {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		num += uint32(b) << ((n - i - 1) << 3)
	}
	return
}

func (r *MemoryReader) Range(yield func([]byte)) {
	if yield != nil {
		for r.Length > 0 {
			yield(r.GetCurrent())
			r.skipBuf()
		}
	} else {
		r.MoveToEnd()
	}
}

func (r *MemoryReader) RangeN(n int, yield func([]byte)) {
	for good := yield != nil; r.Length > 0 && n > 0; r.skipBuf() {
		curBuf := r.GetCurrent()
		if curBufLen := len(curBuf); curBufLen > n {
			if r.forward(n); good {
				yield(curBuf[:n])
			}
			return
		} else if n -= curBufLen; good {
			yield(curBuf)
		}
	}
}

func (r *MemoryReader) ClipFront(yield func([]byte) bool) {
	offset := r.Size - r.Length
	if offset == 0 {
		return
	}
	if m := r.Memory; r.Length == 0 {
		for _, buf := range m.Buffers {
			yield(buf)
		}
		m.Buffers = m.Buffers[:0]
	} else {
		for _, buf := range m.Buffers[:r.offset0] {
			yield(buf)
		}
		if r.offset1 > 0 {
			yield(m.Buffers[r.offset0][:r.offset1])
			m.Buffers[r.offset0] = r.GetCurrent()
		}
		if r.offset0 > 0 {
			m.Buffers = slices.Delete(m.Buffers, 0, r.offset0)
		}
	}
	r.Size -= offset
	r.offset0 = 0
	r.offset1 = 0
}
