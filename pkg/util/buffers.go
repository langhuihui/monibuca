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
	level1 := buffers.GetLevel1()
	if len(level1) == 1 {
		defer buffers.move0()
	} else {
		defer buffers.move1(1)
	}
	return level1[0], nil
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

func (buffers *Buffers) GetLevel0() []byte {
	return buffers.Buffers[buffers.offset0]
}

func (buffers *Buffers) GetLevel1() []byte {
	return buffers.GetLevel0()[buffers.offset1:]
}

func (buffers *Buffers) Skip(n int) error {
	if n > buffers.Length {
		return io.EOF
	}
	for n > 0 {
		level1 := buffers.GetLevel1()
		level1Len := len(level1)
		if n < level1Len {
			buffers.move1(n)
			break
		}
		n -= level1Len
		buffers.move0()
		if buffers.Length == 0 && n > 0 {
			return io.EOF
		}
	}
	return nil
}

func (buffers *Buffers) move1(n int) {
	buffers.offset1 += n
	buffers.Length -= n
	buffers.Offset += n
}

func (buffers *Buffers) move0() {
	len0 := len(buffers.GetLevel0())
	buffers.Offset += len0
	buffers.Length -= len0
	buffers.offset0++
	buffers.offset1 = 0
}

func (buffers *Buffers) ReadBytes(n int) ([]byte, error) {
	if n > buffers.Length {
		return nil, io.EOF
	}
	l := n
	b := make([]byte, n)
	for n > 0 {
		level1 := buffers.GetLevel1()
		level1Len := len(level1)
		if n < level1Len {
			copy(b[l-n:], level1[:n])
			buffers.move1(n)
			break
		}
		copy(b[l-n:], level1)
		n -= level1Len
		buffers.move0()
		if buffers.Length == 0 && n > 0 {
			return nil, io.EOF
		}
	}
	return b, nil
}

func (buffers *Buffers) WriteNTo(n int, result *net.Buffers) (actual int) {
	for actual = n; buffers.Length > 0 && n > 0; buffers.move0() {
		level1 := buffers.GetLevel1()
		remain1 := len(level1)
		if remain1 > n {
			*result = append(*result, level1[:n])
			buffers.move1(n)
			return actual
		}
		*result = append(*result, level1)
		n -= remain1
	}
	return actual - n
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

func (buffers *Buffers) ToBytes() []byte {
	ret := make([]byte, buffers.Length)
	buffers.Read(ret)
	buffers.offset0 = 0
	buffers.offset1 = 0
	buffers.Offset = 0
	buffers.Length = 0
	return ret
}
