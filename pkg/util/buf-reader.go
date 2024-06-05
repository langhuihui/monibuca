package util

import (
	"io"
)

const defaultBufSize = 65536

type BufReader struct {
	reader    io.Reader
	allocator *ScalableMemoryAllocator
	buf       MemoryReader
	BufLen    int
}

func NewBufReaderWithBufLen(reader io.Reader, bufLen int) (r *BufReader) {
	r = &BufReader{}
	r.reader = reader
	r.allocator = NewScalableMemoryAllocator(bufLen)
	r.BufLen = bufLen
	return
}

func NewBufReader(reader io.Reader) (r *BufReader) {
	r = &BufReader{}
	r.reader = reader
	r.allocator = NewScalableMemoryAllocator(defaultBufSize)
	r.BufLen = defaultBufSize
	return
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
		return err
	} else if n < r.BufLen {
		r.buf.ReadFromBytes(buf[:n])
		r.allocator.Free(buf[n:])
	} else if n == r.BufLen {
		r.buf.ReadFromBytes(buf)
	}
	return nil
}

func (r *BufReader) ReadByte() (b byte, err error) {
	for ; r.buf.Length == 0 && err == nil; err = r.eat() {

	}
	if err == nil {
		b, err = r.buf.ReadByte()
	}
	return
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

func (r *BufReader) ReadBytes(n int) (mem RecyclableMemory, err error) {
	mem.ScalableMemoryAllocator = r.allocator
	for r.recycleFront(); n > 0 && err == nil; err = r.eat() {
		if r.buf.Length > 0 {
			if r.buf.Length >= n {
				mem.AddRecycleBytes(r.buf.ClipN(n)...)
				return
			}
			n -= r.buf.Length
			mem.AddRecycleBytes(r.buf.Memory.Buffers...)
			r.buf = MemoryReader{}
		}
	}
	return
}

func (r *BufReader) recycleFront() {
	for _, buf := range r.buf.ClipFront() {
		r.allocator.Free(buf)
	}
}
