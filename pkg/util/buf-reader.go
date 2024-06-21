package util

import (
	"io"
)

const defaultBufSize = 1 << 14

type BufReader struct {
	reader    io.Reader
	Allocator *ScalableMemoryAllocator
	buf       MemoryReader
	BufLen    int
}

func NewBufReaderWithBufLen(reader io.Reader, bufLen int) (r *BufReader) {
	r = &BufReader{
		reader:    reader,
		Allocator: NewScalableMemoryAllocator(bufLen),
		BufLen:    bufLen,
	}
	r.buf.Memory = &Memory{}
	//fmt.Println("NewBufReaderWithBufLen", uintptr(unsafe.Pointer(r.allocator)))
	return
}

func NewBufReader(reader io.Reader) (r *BufReader) {
	return NewBufReaderWithBufLen(reader, defaultBufSize)
}

func (r *BufReader) Recycle() {
	r.reader = nil
	r.buf = MemoryReader{}
	r.Allocator.Recycle()
}

func (r *BufReader) Peek(n int) (buf []byte, err error) {
	defer func(snap MemoryReader) {
		l := r.buf.Length + n
		r.buf = snap
		r.buf.Length = l
	}(
		r.buf)
	for range n {
		if b, err := r.ReadByte(); err != nil {
			return nil, err
		} else {
			buf = append(buf, b)
		}
	}
	return
}

func (r *BufReader) eat() error {
	buf := r.Allocator.Malloc(r.BufLen)
	if n, err := r.reader.Read(buf); err != nil {
		r.Allocator.Free(buf)
		return err
	} else {
		if n < r.BufLen {
			r.Allocator.Free(buf[n:])
			buf = buf[:n]
		}
		r.buf.Buffers = append(r.buf.Buffers, buf)
		r.buf.Size += n
		r.buf.Length += n
	}
	return nil
}

func (r *BufReader) ReadByte() (b byte, err error) {
	for r.buf.Length == 0 {
		if err = r.eat(); err != nil {
			return
		}
	}
	return r.buf.ReadByte()
}

func (r *BufReader) ReadBE(n int) (num int, err error) {
	for i := range n {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		num += int(b) << ((n - i - 1) << 3)
	}
	return
}

func (r *BufReader) ReadLE32(n int) (num uint32, err error) {
	for i := range n {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		num += uint32(b) << (i << 3)
	}
	return
}

func (r *BufReader) ReadBE32(n int) (num uint32, err error) {
	for i := range n {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		num += uint32(b) << ((n - i - 1) << 3)
	}
	return
}

func (r *BufReader) Skip(n int) (err error) {
	return r.ReadRange(n, nil)
}

func (r *BufReader) ReadRange(n int, yield func([]byte)) (err error) {
	for r.recycleFront(); n > 0 && err == nil; err = r.eat() {
		if r.buf.Length > 0 {
			if r.buf.Length >= n {
				r.buf.RangeN(n, yield)
				return
			}
			n -= r.buf.Length
			if yield != nil {
				r.buf.Range(yield)
			}
			r.buf.MoveToEnd()
		}
	}
	return
}

func (r *BufReader) ReadNto(n int, to []byte) (err error) {
	l := 0
	return r.ReadRange(n, func(buf []byte) {
		ll := len(buf)
		copy(to[l:l+ll], buf)
		l += ll
	})
}
func (r *BufReader) ReadString(n int) (s string, err error) {
	err = r.ReadRange(n, func(buf []byte) {
		s += string(buf)
	})
	return
}
func (r *BufReader) ReadBytes(n int) (mem Memory, err error) {
	err = r.ReadRange(n, func(buf []byte) {
		mem.Buffers = append(mem.Buffers, buf)
	})
	mem.Size = n
	return
}

func (r *BufReader) recycleFront() {
	r.buf.ClipFront(r.Allocator.Free)
}
