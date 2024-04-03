package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
)

type Integer interface {
	~int | ~int16 | ~int32 | ~int64 | ~uint | ~uint16 | ~uint32 | ~uint64
}

func PutBE[T Integer](b []byte, num T) []byte {
	for i, n := 0, len(b); i < n; i++ {
		b[i] = byte(num >> ((n - i - 1) << 3))
	}
	return b
}

func ReadBE[T Integer](b []byte) (num T) {
	num = 0
	for i, n := 0, len(b); i < n; i++ {
		num += T(b[i]) << ((n - i - 1) << 3)
	}
	return
}

func GetBE[T Integer](b []byte, num *T) T {
	*num = 0
	for i, n := 0, len(b); i < n; i++ {
		*num += T(b[i]) << ((n - i - 1) << 3)
	}
	return *num
}

// Buffer 用于方便自动扩容的内存写入，已经读取
type Buffer []byte

// ReuseBuffer 重用buffer，内容可能会被覆盖，要尽早复制
type ReuseBuffer struct {
	Buffer
}

func (ReuseBuffer) Reuse() bool {
	return true
}

// LimitBuffer 限制buffer的长度，不会改变原来的buffer，防止内存泄漏
type LimitBuffer struct {
	Buffer
}

func (b *LimitBuffer) ReadN(n int) (result LimitBuffer) {
	result.Buffer = b.Buffer.ReadN(n)
	return
}

func (b LimitBuffer) Clone() (result LimitBuffer) {
	result.Buffer = b.Buffer.Clone()
	return
}

func (b LimitBuffer) SubBuf(start int, length int) (result LimitBuffer) {
	result.Buffer = b.Buffer.SubBuf(start, length)
	return
}

func (b *LimitBuffer) Malloc(count int) (result LimitBuffer) {
	l := b.Len()
	newL := l + count
	if c := b.Cap(); newL > c {
		panic(fmt.Sprintf("LimitBuffer Malloc %d > %d", newL, c))
	} else {
		*b = b.SubBuf(0, newL)
	}
	return b.SubBuf(l, count)
}

func (b *LimitBuffer) Write(a []byte) (n int, err error) {
	l := b.Len()
	newL := l + len(a)
	if c := b.Cap(); newL > c {
		return 0, fmt.Errorf("LimitBuffer Write %d > %d", newL, c)
		// panic(fmt.Sprintf("LimitBuffer Write %d > %d", newL, c))
	} else {
		b.Buffer = b.Buffer.SubBuf(0, newL)
		copy(b.Buffer[l:], a)
	}
	return len(a), nil
}

// IBytes 用于区分传入的内存是否是复用内存，例如从网络中读取的数据，如果是复用内存，需要尽早复制
type IBytes interface {
	Len() int
	Bytes() []byte
	Reuse() bool
}
type IBuffer interface {
	Len() int
	Bytes() []byte
	Reuse() bool
	SubBuf(start int, length int) Buffer
	Malloc(count int) Buffer
	Reset()
	WriteUint32(v uint32)
	WriteUint24(v uint32)
	WriteUint16(v uint16)
	WriteFloat64(v float64)
	WriteByte(v byte)
	WriteString(a string)
	Write(a []byte) (n int, err error)
	ReadN(n int) Buffer
	ReadFloat64() float64
	ReadUint64() uint64
	ReadUint32() uint32
	ReadUint24() uint32
	ReadUint16() uint16
	ReadByte() byte
	Read(buf []byte) (n int, err error)
	Clone() Buffer
	CanRead() bool
	CanReadN(n int) bool
	Cap() int
}

func (Buffer) Reuse() bool {
	return false
}

func (b *Buffer) Read(buf []byte) (n int, err error) {
	if !b.CanReadN(len(buf)) {
		copy(buf, *b)
		return b.Len(), io.EOF
	}
	ret := b.ReadN(len(buf))
	copy(buf, ret)
	return len(ret), err
}

func (b *Buffer) ReadN(n int) Buffer {
	l := b.Len()
	if n > l {
		n = l
	}
	r := (*b)[:n]
	*b = (*b)[n:l]
	return r
}

func (b *Buffer) ReadFloat64() float64 {
	return math.Float64frombits(b.ReadUint64())
}
func (b *Buffer) ReadUint64() uint64 {
	return binary.BigEndian.Uint64(b.ReadN(8))
}
func (b *Buffer) ReadUint32() uint32 {
	return binary.BigEndian.Uint32(b.ReadN(4))
}
func (b *Buffer) ReadUint24() uint32 {
	return ReadBE[uint32](b.ReadN(3))
}
func (b *Buffer) ReadUint16() uint16 {
	return binary.BigEndian.Uint16(b.ReadN(2))
}
func (b *Buffer) ReadByte() byte {
	return b.ReadN(1)[0]
}
func (b *Buffer) WriteFloat64(v float64) {
	PutBE(b.Malloc(8), math.Float64bits(v))
}
func (b *Buffer) WriteUint32(v uint32) {
	binary.BigEndian.PutUint32(b.Malloc(4), v)
}
func (b *Buffer) WriteUint24(v uint32) {
	PutBE(b.Malloc(3), v)
}
func (b *Buffer) WriteUint16(v uint16) {
	binary.BigEndian.PutUint16(b.Malloc(2), v)
}
func (b *Buffer) WriteByte(v byte) {
	b.Malloc(1)[0] = v
}
func (b *Buffer) WriteString(a string) {
	*b = append(*b, a...)
}
func (b *Buffer) Write(a []byte) (n int, err error) {
	l := b.Len()
	newL := l + len(a)
	if newL > b.Cap() {
		*b = append(*b, a...)
	} else {
		*b = b.SubBuf(0, newL)
		copy((*b)[l:], a)
	}
	return len(a), nil
}

func (b Buffer) Clone() (result Buffer) {
	return append(result, b...)
}

func (b Buffer) Bytes() []byte {
	return b
}

func (b Buffer) Len() int {
	return len(b)
}

func (b Buffer) CanRead() bool {
	return b.CanReadN(1)
}

func (b Buffer) CanReadN(n int) bool {
	return b.Len() >= n
}
func (b Buffer) Cap() int {
	return cap(b)
}
func (b Buffer) SubBuf(start int, length int) Buffer {
	return b[start : start+length]
}

// Malloc 扩大原来的buffer的长度，返回新增的buffer
func (b *Buffer) Malloc(count int) Buffer {
	l := b.Len()
	newL := l + count
	if newL > b.Cap() {
		n := make(Buffer, newL)
		copy(n, *b)
		*b = n
	} else {
		*b = b.SubBuf(0, newL)
	}
	return b.SubBuf(l, count)
}

// Relloc 改变 buffer 到指定大小
func (b *Buffer) Relloc(count int) {
	b.Reset()
	b.Malloc(count)
}

func (b *Buffer) Reset() {
	*b = b.SubBuf(0, 0)
}

func (b *Buffer) Split(n int) (result net.Buffers) {
	origin := *b
	for {
		if b.CanReadN(n) {
			result = append(result, b.ReadN(n))
		} else {
			result = append(result, *b)
			*b = origin
			return
		}
	}
}

// ConcatBuffers 合并碎片内存为一个完整内存
func ConcatBuffers[T ~[]byte](input []T) (out []byte) {
	for _, v := range input {
		out = append(out, v...)
	}
	return
}

// SizeOfBuffers 计算Buffers的内容长度
func SizeOfBuffers[T ~[]byte](buf []T) (size int) {
	for _, b := range buf {
		size += len(b)
	}
	return
}

// SplitBuffers 按照一定大小分割 Buffers
func SplitBuffers[T ~[]byte](buf []T, size int) (result [][]T) {
	buf = append([]T(nil), buf...)
	for total := SizeOfBuffers(buf); total > 0; {
		if total <= size {
			return append(result, buf)
		} else {
			var before []T
			sizeOfBefore := 0
			for _, b := range buf {
				need := size - sizeOfBefore
				if lenOfB := len(b); lenOfB > need {
					before = append(before, b[:need])
					result = append(result, before)
					total -= need
					buf[0] = b[need:]
					break
				} else {
					sizeOfBefore += lenOfB
					before = append(before, b)
					total -= lenOfB
					buf = buf[1:]
				}
			}
		}
	}
	return
}
