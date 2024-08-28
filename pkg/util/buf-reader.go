package util

import (
	"io"
	"net"
	"net/textproto"
	"strings"
)

const defaultBufSize = 1 << 14

type BufReader struct {
	Allocator *ScalableMemoryAllocator
	buf       MemoryReader
	BufLen    int
	feedData  func() error
}

func NewBufReaderWithBufLen(reader io.Reader, bufLen int) (r *BufReader) {
	r = &BufReader{
		Allocator: NewScalableMemoryAllocator(bufLen),
		BufLen:    bufLen,
		feedData: func() error {
			buf, err := r.Allocator.Read(reader, r.BufLen)
			if err != nil {
				return err
			}
			n := len(buf)
			r.buf.Buffers = append(r.buf.Buffers, buf)
			r.buf.Size += n
			r.buf.Length += n
			return nil
		},
	}
	r.buf.Memory = &Memory{}
	//fmt.Println("NewBufReaderWithBufLen", uintptr(unsafe.Pointer(r.allocator)))
	return
}

func NewBufReaderBuffersChan(feedChan chan net.Buffers) (r *BufReader) {
	r = &BufReader{
		Allocator: NewScalableMemoryAllocator(defaultBufSize),
		BufLen:    defaultBufSize,
		feedData: func() error {
			data, ok := <-feedChan
			if !ok {
				return io.EOF
			}
			for _, buf := range data {
				n := len(buf)
				r.buf.Size += n
				r.buf.Length += n
			}
			r.buf.Buffers = append(r.buf.Buffers, data...)
			return nil
		},
	}
	r.buf.Memory = &Memory{}
	return
}

func NewBufReaderChan(feedChan chan []byte) (r *BufReader) {
	r = &BufReader{
		Allocator: NewScalableMemoryAllocator(defaultBufSize),
		BufLen:    defaultBufSize,
		feedData: func() error {
			data, ok := <-feedChan
			if !ok {
				return io.EOF
			}
			n := len(data)
			r.buf.Buffers = append(r.buf.Buffers, data)
			r.buf.Size += n
			r.buf.Length += n
			return nil
		},
	}
	r.buf.Memory = &Memory{}
	return
}

func NewBufReader(reader io.Reader) (r *BufReader) {
	return NewBufReaderWithBufLen(reader, defaultBufSize)
}

func (r *BufReader) Recycle() {
	r.buf = MemoryReader{}
	r.Allocator.Recycle()
}

func (r *BufReader) Buffered() int {
	return r.buf.Length
}

func (r *BufReader) Peek(n int) (buf []byte, err error) {
	defer func(snap MemoryReader) {
		l := r.buf.Length + n
		r.buf = snap
		r.buf.Length = l
	}(
		r.buf)
	for range n {
		if b, err := r.ReadByte(); err != nil {
			return nil, err
		} else {
			buf = append(buf, b)
		}
	}
	return
}

func (r *BufReader) ReadByte() (b byte, err error) {
	for r.buf.Length == 0 {
		if err = r.feedData(); err != nil {
			return
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

func (r *BufReader) Skip(n int) (err error) {
	return r.ReadRange(n, nil)
}

func (r *BufReader) ReadRange(n int, yield func([]byte)) (err error) {
	for r.recycleFront(); n > 0 && err == nil; err = r.feedData() {
		if r.buf.Length > 0 {
			if r.buf.Length >= n {
				r.buf.RangeN(n, yield)
				return
			}
			n -= r.buf.Length
			r.buf.Range(yield)
		}
	}
	return
}

func (r *BufReader) Read(to []byte) (n int, err error) {
	n = len(to)
	err = r.ReadNto(n, to)
	return
}

func (r *BufReader) ReadNto(n int, to []byte) (err error) {
	l := 0
	return r.ReadRange(n, func(buf []byte) {
		ll := len(buf)
		copy(to[l:l+ll], buf)
		l += ll
	})
}

func (r *BufReader) ReadString(n int) (s string, err error) {
	err = r.ReadRange(n, func(buf []byte) {
		s += string(buf)
	})
	return
}

func (r *BufReader) ReadBytes(n int) (mem Memory, err error) {
	err = r.ReadRange(n, mem.AppendOne)
	return
}

func (r *BufReader) recycleFront() {
	r.buf.ClipFront(r.Allocator.Free)
}

func (r *BufReader) ReadLine() (line string, err error) {
	var lastb, curb byte
	snap, i := r.buf, 0
	for {
		if curb, err = r.ReadByte(); err != nil {
			return "", err
		} else {
			i++
			if l := r.buf.Length; curb == '\n' {
				snap.Length = l + i
				r.buf = snap
				err = r.ReadRange(i, func(buf []byte) {
					line = line + string(buf)
				})
				if lastb == '\r' {
					line = line[:i-2]
				} else {
					line = line[:i-1]
				}
				return
			}
			lastb = curb
		}
	}
}

func (r *BufReader) ReadMIMEHeader() (textproto.MIMEHeader, error) {
	result := make(textproto.MIMEHeader)
	for {
		l, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		if l == "" {
			break
		}
		key, value, _ := strings.Cut(l, ":")
		result.Add(key, strings.Trim(value, " "))
	}
	return result, nil
}
