package util

import (
	"io"
)

const defaultBufSize = 65536

type BufReader struct {
	reader io.Reader
	buf    RecyclableBuffers
	BufLen int
}

func NewBufReaderWithBufLen(reader io.Reader, bufLen int) (r *BufReader) {
	r = &BufReader{}
	r.reader = reader
	r.buf.ScalableMemoryAllocator = NewScalableMemoryAllocator(bufLen)
	r.BufLen = bufLen
	return
}

func NewBufReader(reader io.Reader) (r *BufReader) {
	r = &BufReader{}
	r.reader = reader
	r.buf.ScalableMemoryAllocator = NewScalableMemoryAllocator(defaultBufSize)
	r.BufLen = defaultBufSize
	return
}

func (r *BufReader) eat() error {
	buf := r.buf.NextN(r.BufLen)
	if n, err := r.reader.Read(buf); err != nil {
		return err
	} else if n < r.BufLen {
		r.buf.RecycleBack(r.BufLen - n)
	}
	return nil
}

func (r *BufReader) ReadByte() (byte, error) {
	for r.buf.Length == 0 {
		err := r.eat()
		if err != nil {
			return 0, err
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

func (r *BufReader) ReadBytes(n int) (mem *RecyclableBuffers, err error) {
	mem = &RecyclableBuffers{ScalableMemoryAllocator: r.buf.ScalableMemoryAllocator}
	for r.buf.RecycleFront(); n > 0 && err == nil; err = r.eat() {
		if r.buf.Length > 0 {
			if r.buf.Length >= n {
				mem.ReadFromBytes(r.buf.Buffers.Cut(n)...)
				return
			}
			n -= r.buf.Length
			mem.ReadFromBytes(r.buf.CutAll()...)
		}
	}
	return
}
