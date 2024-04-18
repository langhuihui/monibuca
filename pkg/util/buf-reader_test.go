package util

import (
	"bytes"
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
