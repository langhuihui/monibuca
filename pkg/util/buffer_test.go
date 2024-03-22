package util

import (
	"testing"
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
