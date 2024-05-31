package util

import (
	"testing"
)

func TestMem(t *testing.T) {
	t.Run(t.Name(), func(t *testing.T) {
		mem := NewMemoryAllocator(65536)
		totalMalloc := 0
		totalFree := 0
		checkSize := func() {
			freeSize := mem.allocator.GetFreeSize()
			if freeSize != mem.allocator.Size-(totalMalloc-totalFree) {
				t.Fail()
			}
		}

		mem.Malloc(65536)
		totalMalloc += 65536
		checkSize()
		mem.free(1536, 64000)
		totalFree += 64000
		checkSize()
		mem.free(0, 1536)
		totalFree += 1536
		checkSize()
		mem.Malloc(65536)
		totalMalloc += 65536
		checkSize()
		mem.free(151, 65385)
		totalFree += 65385
		checkSize()
		mem.free(0, 12)
		totalFree += 12
		checkSize()
		mem.free(140, 1)
		totalFree += 1
		checkSize()
		mem.free(12, 128)
		totalFree += 128
		checkSize()
	})
}
