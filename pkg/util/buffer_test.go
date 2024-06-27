package util

import (
	"math/rand"
	"testing"
	"time"
)

func TestBuffer(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		var b Buffer
		t.Log(b == nil)
		b.Write([]byte{1, 2, 3})
		if b == nil {
			t.Fail()
		} else {
			t.Logf("b:% x", b)
		}
	})
}

func TestReadBytesTo(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		s := RandomString(100)
		t.Logf("s:%s", s)
		var m Memory
		m.Append([]byte(s))
		r := m.NewReader()
		seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
		var total []byte
		for {
			i := seededRand.Intn(10)
			if i == 0 {
				continue
			}
			buf := make([]byte, i)
			n := r.ReadBytesTo(buf)
			t.Logf("n:%d buf:%s", n, string(buf))
			total = append(total, buf[:n]...)
			if n == 0 {
				if string(total) != s {
					t.Logf("total:%s", total)
					t.Fail()
				}
				return
			}
		}
	})
}
