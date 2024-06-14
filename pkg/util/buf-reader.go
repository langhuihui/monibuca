package util

import (
	"io"
)

const defaultBufSize = 1 << 14

type BufReader struct {
	reader    io.Reader
	allocator *ScalableMemoryAllocator
	buf       MemoryReader
	BufLen    int
	Err       error
}

func NewBufReaderWithBufLen(reader io.Reader, bufLen int) (r *BufReader) {
	r = &BufReader{
		reader:    reader,
		allocator: NewScalableMemoryAllocator(bufLen),
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
	r.allocator.Recycle()
}

func (r *BufReader) eat() error {
	buf := r.allocator.Malloc(r.BufLen)
	if n, err := r.reader.Read(buf); err != nil {
		r.allocator.Free(buf)
		r.Err = err
		return err
	} else {
		if n < r.BufLen {
			r.allocator.Free(buf[n:])
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
	r.recycleFront()
	for r.buf.Length < n {
		if err = r.eat(); err != nil {
			return err
		}
	}
	r.buf.RangeN(n)(nil)
	return
}

func (r *BufReader) ReadRange(n int) func(func([]byte) bool) {
	return func(yield func([]byte) bool) {
		for r.recycleFront(); n > 0 && r.Err == nil; r.eat() {
			if r.buf.Length > 0 {
				if r.buf.Length >= n {
					r.buf.RangeN(n)(yield)
					return
				}
				n -= r.buf.Length
				for _, buf := range r.buf.Buffers {
					yield(buf)
				}
				r.buf.MoveToEnd()
			}
		}
	}
}

func (r *BufReader) ReadBytes(n int) (mem Memory, err error) {
	for buf := range r.ReadRange(n) {
		mem.Buffers = append(mem.Buffers, buf)
	}
	mem.Size = n
	return mem, r.Err
}

func (r *BufReader) recycleFront() {
	r.buf.ClipFront(r.allocator.Free)
}
