package util

import (
	"io"
	"net"
	"slices"
)

type Memory struct {
	Size int
	net.Buffers
}

type MemoryReader struct {
	Memory
	Length  int
	offset0 int
	offset1 int
}

func NewMemoryFromBytes(b ...[]byte) *Memory {
	return NewMemory(b)
}

func NewReadableBuffersFromBytes(b ...[]byte) *MemoryReader {
	buf := NewMemory(b)
	return &MemoryReader{Memory: *buf, Length: buf.Size}
}

func NewMemory(buffers net.Buffers) *Memory {
	ret := &Memory{Buffers: buffers}
	for _, level0 := range buffers {
		ret.Size += len(level0)
	}
	return ret
}

func (buffers *Memory) UpdateBuffer(index int, buf []byte) {
	if index < 0 {
		index = len(buffers.Buffers) + index
	}
	buffers.Size = len(buf) - len(buffers.Buffers[index])
	buffers.Buffers[index] = buf
}

func (buffers *Memory) CopyFrom(b Memory) {
	buf := make([]byte, b.Size)
	bufs := slices.Clone(b.Buffers)
	bufs.Read(buf)
	buffers.ReadFromBytes(buf)
}

func (buffers *Memory) ReadFromBytes(b ...[]byte) {
	buffers.Buffers = append(buffers.Buffers, b...)
	for _, level0 := range b {
		buffers.Size += len(level0)
	}
}

func (buffers *Memory) Count() int {
	return len(buffers.Buffers)
}

func (r Memory) NewReader() *MemoryReader {
	var reader MemoryReader
	reader.Memory = r
	reader.Length = r.Size
	return &reader
}

func (buffers *MemoryReader) ReadFromBytes(b ...[]byte) {
	buffers.Memory.Buffers = append(buffers.Memory.Buffers, b...)
	for _, level0 := range b {
		buffers.Size += len(level0)
		buffers.Length += len(level0)
	}
}

func (buffers *MemoryReader) Pop() []byte {
	panic("ReadableBuffers Pop not allowed")
}

func (buffers *MemoryReader) GetCurrent() []byte {
	return buffers.Memory.Buffers[buffers.offset0][buffers.offset1:]
}

func (buffers *MemoryReader) MoveToEnd() {
	buffers.offset0 = buffers.Count()
	buffers.offset1 = 0
	buffers.Length = 0
}

func (buffers *MemoryReader) ReadBytesTo(buf []byte) (actual int) {
	n := len(buf)
	curBuf := buffers.GetCurrent()
	curBufLen := len(curBuf)
	if n > buffers.Length {
		if curBufLen > 0 {
			actual += copy(buf, curBuf)
			buffers.offset0++
			buffers.offset1 = 0
		}
		for _, b := range buffers.Memory.Buffers[buffers.offset0:] {
			actual += copy(buf[actual:], b)
		}
		buffers.MoveToEnd()
		return
	}
	l := n
	for n > 0 {
		if n < curBufLen {
			actual += n
			copy(buf[l-n:], curBuf[:n])
			buffers.forward(n)
			break
		}
		copy(buf[l-n:], curBuf)
		n -= curBufLen
		actual += curBufLen
		buffers.skipBuf()
		if buffers.Length == 0 && n > 0 {
			return
		}
	}
	return
}
func (reader *MemoryReader) ReadByteTo(b ...*byte) (err error) {
	for i := range b {
		if reader.Length == 0 {
			return io.EOF
		}
		*b[i], err = reader.ReadByte()
		if err != nil {
			return
		}
	}
	return
}

func (reader *MemoryReader) ReadByteMask(mask byte) (byte, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	return b & mask, nil
}

func (reader *MemoryReader) ReadByte() (b byte, err error) {
	if reader.Length == 0 {
		return 0, io.EOF
	}
	curBuf := reader.GetCurrent()
	b = curBuf[0]
	if len(curBuf) == 1 {
		reader.skipBuf()
	} else {
		reader.forward(1)
	}
	return
}

func (reader *MemoryReader) LEB128Unmarshal() (uint, int, error) {
	v := uint(0)
	n := 0
	for i := 0; i < 8; i++ {
		b, err := reader.ReadByte()
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
func (reader *MemoryReader) getCurrentBufLen() int {
	return len(reader.Memory.Buffers[reader.offset0]) - reader.offset1
}
func (reader *MemoryReader) Skip(n int) error {
	if n > reader.Length {
		return io.EOF
	}
	curBufLen := reader.getCurrentBufLen()
	for n > 0 {
		if n < curBufLen {
			reader.forward(n)
			break
		}
		n -= curBufLen
		reader.skipBuf()
		if reader.Length == 0 && n > 0 {
			return io.EOF
		}
	}
	return nil
}

func (reader *MemoryReader) forward(n int) {
	reader.Length -= n
	reader.offset1 += n
}

func (buffers *MemoryReader) skipBuf() {
	curBufLen := buffers.getCurrentBufLen()
	buffers.Length -= curBufLen
	buffers.offset0++
	buffers.offset1 = 0
}

func (reader *MemoryReader) ReadBytes(n int) ([]byte, error) {
	if n > reader.Length {
		return nil, io.EOF
	}
	b := make([]byte, n)
	actual := reader.ReadBytesTo(b)
	return b[:actual], nil
}

// func (buffers *ReadableBuffers) WriteTo(w io.Writer) (n int64, err error) {
// 	var buf net.Buffers
// 	if buffers.Count() > buffers.offset1 {
// 		buf = append(buf, buffers.Buffers[buffers.offset:]...)
// 	}
// 	if buffers.curBufLen > 0 {
// 		buf[0] = buffers.curBuf
// 	}
// 	buffers.MoveToEnd()
// 	return buf.WriteTo(w)
// }

func (reader *MemoryReader) WriteNTo(n int, result *net.Buffers) (actual int) {
	for actual = n; reader.Length > 0 && n > 0; reader.skipBuf() {
		curBuf := reader.GetCurrent()
		if len(curBuf) > n {
			if result != nil {
				*result = append(*result, curBuf[:n])
			}
			reader.forward(n)
			return actual
		}
		if result != nil {
			*result = append(*result, curBuf)
		}
		n -= len(curBuf)
	}
	return actual - n
}

func (reader *MemoryReader) ReadBE(n int) (num int, err error) {
	for i := range n {
		b, err := reader.ReadByte()
		if err != nil {
			return -1, err
		}
		num += int(b) << ((n - i - 1) << 3)
	}
	return
}

func (reader *MemoryReader) ClipN(n int) (r net.Buffers) {
	reader.WriteNTo(n, nil)
	return reader.ClipFront()
}

func (reader *MemoryReader) ClipFront() (r net.Buffers) {
	offset := reader.Size - reader.Length
	if offset == 0 {
		return
	}
	buffers := &reader.Memory
	if reader.Length == 0 {
		r = slices.Clone(buffers.Buffers)
		buffers.Buffers = buffers.Buffers[:0]
	} else {
		r = slices.Clone(buffers.Buffers[:reader.offset0])
		if reader.offset1 > 0 {
			r = append(r, buffers.Buffers[reader.offset0][:reader.offset1])
			buffers.Buffers[reader.offset0] = reader.GetCurrent()
		}
		if reader.offset0 > 0 {
			buffers.Buffers = slices.Delete(buffers.Buffers, 0, reader.offset0)
		}
	}
	// bs := 0
	// for _, b := range r {
	// 	bs += len(b)
	// }
	// if bs != offset {
	// 	panic("ClipFront error")
	// }
	reader.Size -= offset
	reader.offset0 = 0
	reader.offset1 = 0
	return
}

func (buffers *Memory) ToBytes() []byte {
	ret := make([]byte, buffers.Size)
	var clone net.Buffers
	clone = append(clone, buffers.Buffers...)
	clone.Read(ret)
	return ret
}
