package util

import (
	"io"
	"net"
	"slices"
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

func (buffers *Buffers) MoveToEnd() {
	buffers.curBuf = nil
	buffers.curBufLen = 0
	buffers.offset = len(buffers.Buffers)
	buffers.Offset = buffers.Length
	buffers.Length = 0
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

func (buffers *Buffers) ReadBytesTo(buf []byte) (actual int) {
	n := len(buf)
	if n > buffers.Length {
		if buffers.curBufLen > 0 {
			actual += copy(buf, buffers.curBuf)
			buffers.offset++
		}
		for _, b := range buffers.Buffers[buffers.offset:] {
			actual += copy(buf[actual:], b)
		}
		buffers.MoveToEnd()
		return
	}
	l := n
	for n > 0 {
		if n < buffers.curBufLen {
			actual += n
			copy(buf[l-n:], buffers.curBuf[:n])
			buffers.forward(n)
			break
		}
		copy(buf[l-n:], buffers.curBuf)
		n -= buffers.curBufLen
		actual += buffers.curBufLen
		buffers.skipBuf()
		if buffers.Length == 0 && n > 0 {
			return
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
	actual := buffers.ReadBytesTo(b)
	return b[:actual], nil
}

func (buffers *Buffers) WriteTo(w io.Writer) (n int64, err error) {
	var buf net.Buffers
	if len(buffers.Buffers) > buffers.offset {
		buf = append(buf, buffers.Buffers[buffers.offset:]...)
	}
	if buffers.curBufLen > 0 {
		buf[0] = buffers.curBuf
	}
	buffers.MoveToEnd()
	return buf.WriteTo(w)
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

func (buffers *Buffers) Consumes() (r net.Buffers) {
	for i := range buffers.offset {
		r = append(r, buffers.Buffers[i])
	}
	if buffers.curBufLen > 0 {
		r = append(r, buffers.curBuf[:len(buffers.curBuf)-buffers.curBufLen])
	}
	return
}

func (buffers *Buffers) ClipFront() (r net.Buffers) {
	if buffers.Offset == 0 {
		return
	}
	if buffers.Length == 0 {
		r = buffers.Buffers
		buffers.Buffers = buffers.Buffers[:0]
		buffers.curBuf = nil
		buffers.curBufLen = 0
		buffers.offset = 0
		buffers.Offset = 0
		return
	}
	for i := range buffers.offset {
		r = append(r, buffers.Buffers[i])
		l := len(buffers.Buffers[i])
		buffers.Offset -= l
	}
	if buffers.curBufLen > 0 {
		l := len(buffers.Buffers[buffers.offset]) - buffers.curBufLen
		r = append(r, buffers.Buffers[buffers.offset][:l])
		buffers.Offset -= l
	}
	buffers.Buffers = buffers.Buffers[buffers.offset:]
	buffers.Buffers[0] = buffers.curBuf
	buffers.offset = 0
	buffers.Offset = 0
	return r
}

func (buffers *Buffers) ClipBack(n int) []byte {
	lastBuf := buffers.Buffers[len(buffers.Buffers)-1]
	lastBufLen := len(lastBuf)
	if lastBufLen < n {
		panic("ClipBack: n > lastBufLen")
	}
	ret := lastBuf[lastBufLen-n:]
	buffers.Buffers[len(buffers.Buffers)-1] = lastBuf[:lastBufLen-n]
	buffers.Length -= n
	if buffers.Length > 0 {
		if buffers.offset == len(buffers.Buffers)-1 {
			buffers.curBuf = buffers.curBuf[:buffers.curBufLen-n]
			buffers.curBufLen -= n
		}
	} else {
		buffers.curBuf = nil
		buffers.curBufLen = 0
		buffers.Length = 0
	}
	return ret
}

func (buffers *Buffers) CutAll() (r net.Buffers) {
	r = append(r, buffers.curBuf)
	for i := buffers.offset + 1; i < len(buffers.Buffers); i++ {
		r = append(r, buffers.Buffers[i])
	}
	if len(buffers.Buffers[buffers.offset]) == buffers.curBufLen {
		buffers.Buffers = buffers.Buffers[:buffers.offset]
	} else {
		buffers.Buffers[buffers.offset] = buffers.Buffers[buffers.offset][:buffers.curBufLen]
		buffers.offset++
	}
	buffers.Length = 0
	buffers.curBuf = nil
	buffers.curBufLen = 0
	return
}

func (buffers *Buffers) Cut(n int) (r net.Buffers) {
	buffers.CutTo(n, &r)
	return
}

func (buffers *Buffers) CutTo(n int, result *net.Buffers) (actual int) {
	for actual = n; buffers.Length > 0 && n > 0; {
		if buffers.curBufLen > n {
			*result = append(*result, buffers.curBuf[:n])
			buffers.curBuf = buffers.curBuf[n:]
			buffers.curBufLen -= n
			buffers.Buffers[buffers.offset] = buffers.curBuf
			buffers.Length -= n
			return actual
		}
		*result = append(*result, buffers.curBuf)
		n -= buffers.curBufLen
		buffers.Length -= buffers.curBufLen
		if len(buffers.Buffers[buffers.offset]) == buffers.curBufLen {
			buffers.Buffers = slices.Delete(buffers.Buffers, buffers.offset, 1)
		} else {
			buffers.Buffers[buffers.offset] = buffers.Buffers[buffers.offset][:buffers.curBufLen]
			buffers.offset++
		}
		if buffers.Length > 0 {
			buffers.curBuf = buffers.Buffers[buffers.offset]
			buffers.curBufLen = len(buffers.curBuf)
		} else {
			buffers.curBuf = nil
			buffers.curBufLen = 0
		}
	}
	return actual - n
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
