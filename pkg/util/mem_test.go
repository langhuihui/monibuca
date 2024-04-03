package util

import (
	"testing"
)

func TestMem(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		mem := NewMemoryAllocator(1024)
		b1 := mem.Malloc(512)
		b2 := mem.Malloc(256)
		b3 := mem.Malloc(256)
		mem.Free(b2)
		mem.Free(b3)
		b2 = mem.Malloc(512)
		if b2 == nil {
			t.Fail()
		}
		mem.Free(b2)
		mem.Free(b1)
		if mem.Malloc(1024) == nil {
			t.Fail()
		}
	})
}
