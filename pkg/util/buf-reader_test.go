package util

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestBufRead(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		var testData = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		testReader := bytes.NewReader(testData)
		reader := NewBufReader(testReader)
		reader.BufLen = 5
		b, err := reader.ReadByte()
		if err != nil {
			t.Error(err)
			return
		}
		if b != 1 {
			t.Error("byte read error")
			return
		}
		num, err := reader.ReadBE(4)
		if err != nil {
			t.Error(err)
			return
		}
		if num != 0x02030405 {
			t.Error("read be error")
			return
		}
		if reader.buf.Length != 0 {
			t.Error("reader.buf.Length != 0")
			return
		}
		b, err = reader.ReadByte()
		if err != nil {
			t.Error(err)
			return
		}
		if b != 6 {
			t.Error("byte read error")
			return
		}
		mem, err := reader.ReadBytes(5)
		if err != nil {
			t.Error(err)
			return
		}
		if len(mem.Buffers.Buffers) != 2 {
			t.Error("len(mem.Buffers.Buffers) != 2")
			return
		}
		if mem.Buffers.Buffers[0][0] != 7 {
			t.Error("mem.Buffers.Buffers[0][0] != 7")
			return
		}
		if mem.Buffers.Buffers[1][0] != 11 {
			t.Error("mem.Buffers.Buffers[1][0] != 10")
			return
		}
		b, err = reader.ReadByte()
		if err != nil {
			t.Error(err)
			return
		}
		if b != 12 {
			t.Error("byte read error")
			return
		}
	})
}
func BenchmarkIoReader(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var testData = make([]byte, 10*1024*1024)
		var err error
		for pb.Next() {
			rand.Read(testData)
			testReader := bytes.NewReader(testData)
			reader := bufio.NewReader(testReader)
			var bb []byte
			for err == nil {
				r := rand.Intn(10)
				if r < 4 {
					_, err = reader.ReadByte()
				} else if r < 7 {
					bb = make([]byte, 4)
					reader.Read(bb)
					binary.BigEndian.Uint32(bb)
				} else {
					bb = make([]byte, rand.Intn(4096))
					_, err = reader.Read(bb)
				}
			}
		}
	})
}

func BenchmarkBufRead(b *testing.B) {
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var testData = make([]byte, 10*1024*1024)
		var err error
		var mem RecyclableBuffers
		for pb.Next() {
			rand.Read(testData)
			testReader := bytes.NewReader(testData)
			reader := NewBufReaderWithBufLen(testReader, 1024)
			for err == nil {
				mem.Recycle()
				r := rand.Intn(10)
				if r < 4 {
					_, err = reader.ReadByte()
				} else if r < 7 {
					_, err = reader.ReadBE(4)
				} else {
					mem, err = reader.ReadBytes(rand.Intn(4096))
				}
			}
		}
	})
}
