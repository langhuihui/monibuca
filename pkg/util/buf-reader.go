package util

import (
	"io"
)

const defaultBufSize = 4096

type BufReader struct {
	reader io.Reader
	buf    RecyclableBuffers
	BufLen int
}

func NewBufReader(reader io.Reader) (r *BufReader) {
	r = &BufReader{}
	r.reader = reader
	r.buf.ScalableMemoryAllocator = NewScalableMemoryAllocator(4096)
	r.BufLen = defaultBufSize
	return
}

func (r *BufReader) eat() error {
	buf := r.buf.Malloc(r.BufLen)
	if n, err := r.reader.Read(buf); err != nil {
		return err
	} else if n < r.BufLen {
		r.buf.RecycleBack(r.BufLen - n)
	}
	return nil
}

func (r *BufReader) ReadByte() (byte, error) {
	if r.buf.Length > 0 {
		return r.buf.ReadByte()
	}
	err := r.eat()
	if err != nil {
		return 0, err
	}
	return r.buf.ReadByte()
}

func (r *BufReader) ReadBE(n int) (num int, err error) {
	for i := range n {
		b, err := r.ReadByte()
		if err != nil {
			return -1, err
		}
		num += int(b) << ((n - i - 1) << 3)
	}
	return
}

func (r *BufReader) ReadBytes(n int) (mem *RecyclableBuffers, err error) {
	mem = &RecyclableBuffers{ScalableMemoryAllocator: r.buf.ScalableMemoryAllocator}
	for r.buf.RecycleFront(); n > 0 && err == nil; err = r.eat() {
		if r.buf.Length >= n {
			mem.ReadFromBytes(r.buf.Buffers.Cut(n)...)
			return
		}
		n -= r.buf.Length
		mem.ReadFromBytes(r.buf.CutAll()...)
	}
	return
}
