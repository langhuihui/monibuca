package util

import (
	"io"
	"net"
	"slices"
)

const (
	MaxBlockSize = 1 << 22
	BuddySize    = MaxBlockSize << 7
	MinPowerOf2  = 10
)

type Memory struct {
	Size    int
	Buffers [][]byte
}

func NewMemory(buf []byte) Memory {
	return Memory{
		Buffers: net.Buffers{buf},
		Size:    len(buf),
	}
}

func (m *Memory) WriteTo(w io.Writer) (n int64, err error) {
	copy := net.Buffers(slices.Clone(m.Buffers))
	return copy.WriteTo(w)
}

func (m *Memory) Reset() {
	m.Buffers = m.Buffers[:0]
	m.Size = 0
}

func (m *Memory) UpdateBuffer(index int, buf []byte) {
	if index < 0 {
		index = len(m.Buffers) + index
	}
	m.Size = len(buf) - len(m.Buffers[index])
	m.Buffers[index] = buf
}

func (m *Memory) CopyFrom(b *Memory) {
	buf := make([]byte, b.Size)
	b.CopyTo(buf)
	m.PushOne(buf)
}

func (m *Memory) Equal(b *Memory) bool {
	if m.Size != b.Size || len(m.Buffers) != len(b.Buffers) {
		return false
	}
	for i, buf := range m.Buffers {
		if !slices.Equal(buf, b.Buffers[i]) {
			return false
		}
	}
	return true
}

func (m *Memory) CopyTo(buf []byte) {
	for _, b := range m.Buffers {
		l := len(b)
		copy(buf, b)
		buf = buf[l:]
	}
}

func (m *Memory) ToBytes() []byte {
	buf := make([]byte, m.Size)
	m.CopyTo(buf)
	return buf
}

func (m *Memory) PushOne(b []byte) {
	m.Buffers = append(m.Buffers, b)
	m.Size += len(b)
}

func (m *Memory) Push(b ...[]byte) {
	m.Buffers = append(m.Buffers, b...)
	for _, level0 := range b {
		m.Size += len(level0)
	}
}

func (m *Memory) Append(mm Memory) *Memory {
	m.Buffers = append(m.Buffers, mm.Buffers...)
	m.Size += mm.Size
	return m
}

func (m *Memory) Count() int {
	return len(m.Buffers)
}

func (m *Memory) Range(yield func([]byte)) {
	for i := range m.Count() {
		yield(m.Buffers[i])
	}
}

func (m *Memory) NewReader() MemoryReader {
	return MemoryReader{
		Memory: m,
		Length: m.Size,
	}
}
