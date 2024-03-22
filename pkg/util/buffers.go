package util

import (
	"io"
	"net"
)

type Buffers struct {
	Offset  int
	offset0 int
	offset1 int
	Length  int
	net.Buffers
}

func NewBuffersFromBytes(b ...[]byte) *Buffers {
	return NewBuffers(net.Buffers(b))
}

func NewBuffers(buffers net.Buffers) *Buffers {
	ret := &Buffers{Buffers: buffers}
	for _, level0 := range buffers {
		ret.Length += len(level0)
	}
	return ret
}

func (buffers *Buffers) ReadFromBytes(b ...[]byte) {
	buffers.Buffers = append(buffers.Buffers, b...)
	for _, level0 := range b {
		buffers.Length += len(level0)
	}
}

func (buffers *Buffers) ReadByteTo(b ...*byte) (err error) {
	for i := range b {
		if buffers.Length == 0 {
			return io.EOF
		}
		*b[i], err = buffers.ReadByte()
		if err != nil {
			return
		}
	}
	return
}

func (buffers *Buffers) ReadByteMask(mask byte) (byte, error) {
	b, err := buffers.ReadByte()
	if err != nil {
		return 0, err
	}
	return b & mask, nil
}

func (buffers *Buffers) ReadByte() (byte, error) {
	if buffers.Length == 0 {
		return 0, io.EOF
	}
	level0 := buffers.Buffers[buffers.offset0]
	b := level0[buffers.offset1]
	buffers.offset1++
	buffers.Length--
	buffers.Offset++
	if buffers.offset1 >= len(level0) {
		buffers.offset0++
		buffers.offset1 = 0
	}
	return b, nil
}

func (buffers *Buffers) LEB128Unmarshal() (uint, int, error) {
	v := uint(0)
	n := 0

	for i := 0; i < 8; i++ {
		b, err := buffers.ReadByte()
		if err != nil {
			return 0, 0, err
		}
		v |= (uint(b&0b01111111) << (i * 7))
		n++

		if (b & 0b10000000) == 0 {
			break
		}
	}

	return v, n, nil
}

func (buffers *Buffers) Skip(n int) error {
	if n > buffers.Length {
		return io.EOF
	}
	buffers.Length -= n
	buffers.Offset += n
	for n > 0 {
		level0 := buffers.Buffers[buffers.offset0]
		level1 := level0[buffers.offset1:]
		if n < len(level1) {
			buffers.offset1 += n
			break
		}
		n -= len(level1)
		buffers.offset0++
		buffers.offset1 = 0
	}
	return nil
}

func (buffers *Buffers) ReadBytes(n int) ([]byte, error) {
	if n > buffers.Length {
		return nil, io.EOF
	}
	b := make([]byte, n)
	buffers.Read(b)
	buffers.Length -= n
	return b, nil
}

func (buffers *Buffers) ReadBE(n int) (num int, err error) {
	for i := range n {
		b, err := buffers.ReadByte()
		if err != nil {
			return -1, err
		}
		num += int(b) << ((n - i - 1) << 3)
	}
	return
}
