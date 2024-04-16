package util

import (
	"io"
	"net"
)

type Buffers struct {
	Offset    int
	offset    int
	Length    int
	curBuf    []byte
	curBufLen int
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
	ret.curBuf = buffers[0]
	ret.curBufLen = len(buffers[0])
	return ret
}

func (buffers *Buffers) ReadFromBytes(b ...[]byte) {
	buffers.Buffers = append(buffers.Buffers, b...)
	for _, level0 := range b {
		buffers.Length += len(level0)
	}
	if buffers.curBuf == nil {
		buffers.curBuf = buffers.Buffers[buffers.offset]
		buffers.curBufLen = len(buffers.curBuf)
	}
}

func (buffers *Buffers) ReadBytesTo(buf []byte) (err error) {
	n := len(buf)
	if n > buffers.Length {
		return io.EOF
	}
	l := n
	for n > 0 {
		if n < buffers.curBufLen {
			copy(buf[l-n:], buffers.curBuf[:n])
			buffers.forward(n)
			break
		}
		copy(buf[l-n:], buffers.curBuf)
		n -= buffers.curBufLen
		buffers.skipBuf()
		if buffers.Length == 0 && n > 0 {
			return io.EOF
		}
	}
	return
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
	if buffers.curBufLen == 1 {
		defer buffers.skipBuf()
	} else {
		defer buffers.forward(1)
	}
	return buffers.curBuf[0], nil
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
	for n > 0 {
		if n < buffers.curBufLen {
			buffers.forward(n)
			break
		}
		n -= buffers.curBufLen
		buffers.skipBuf()
		if buffers.Length == 0 && n > 0 {
			return io.EOF
		}
	}
	return nil
}

func (buffers *Buffers) forward(n int) {
	buffers.curBuf = buffers.curBuf[n:]
	buffers.curBufLen -= n
	buffers.Length -= n
	buffers.Offset += n
}

func (buffers *Buffers) skipBuf() {
	buffers.Offset += buffers.curBufLen
	buffers.Length -= buffers.curBufLen
	buffers.offset++
	if buffers.Length > 0 {
		buffers.curBuf = buffers.Buffers[buffers.offset]
		buffers.curBufLen = len(buffers.curBuf)
	} else {
		buffers.curBuf = nil
		buffers.curBufLen = 0
	}
}

func (buffers *Buffers) ReadBytes(n int) ([]byte, error) {
	if n > buffers.Length {
		return nil, io.EOF
	}
	b := make([]byte, n)
	err := buffers.ReadBytesTo(b)
	return b, err
}

func (buffers *Buffers) WriteNTo(n int, result *net.Buffers) (actual int) {
	for actual = n; buffers.Length > 0 && n > 0; buffers.skipBuf() {
		if buffers.curBufLen > n {
			*result = append(*result, buffers.curBuf[:n])
			buffers.forward(n)
			return actual
		}
		*result = append(*result, buffers.curBuf)
		n -= buffers.curBufLen
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
	buffers.offset = 0
	buffers.Offset = 0
	buffers.Length = 0
	buffers.curBuf = nil
	return ret
}
