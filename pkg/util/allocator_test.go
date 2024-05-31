package util

import (
	"slices"
	"testing"
)

func TestAllocator(t *testing.T) {
	allocator := NewAllocator(1000)

	// 分配内存
	block1 := allocator.Allocate(100)
	if block1 != 0 {
		t.Error("Failed to allocate memory")
	}

	// 分配内存
	block2 := allocator.Allocate(200)
	if block2 != 100 {
		t.Error("Failed to allocate memory")
	}

	// 释放内存
	allocator.Free(0, 299)
	if allocator.GetFreeSize() != 999 {
		t.Error("Failed to free memory")
	}
	allocator.Free(299, 1)

	// 重新分配内存
	block3 := allocator.Allocate(50)
	if block3 != 0 {
		t.Error("Failed to allocate memory")
	}

	// 释放内存
	allocator.Free(0, 50)

	// 分配大于剩余空间的内存
	block4 := allocator.Allocate(1000)
	if block4 != 0 {
		t.Error("Should not allocate memory larger than available space")
	}
}

func FuzzAllocator(f *testing.F) {
	f.Add(100, false)
	allocator := NewAllocator(65535)
	var used [][2]int
	var totalMalloc, totalFree int = 0, 0
	f.Fuzz(func(t *testing.T, size int, alloc bool) {
		free := !alloc
		if size <= 0 {
			return
		}
		t.Logf("totalFree:%d,size:%d, free:%v", totalFree, size, free)
		defer func() {
			t.Logf("totalMalloc:%d, totalFree:%d, freeSize:%d", totalMalloc, totalFree, allocator.GetFreeSize())
			if totalMalloc-totalFree != allocator.Size-allocator.GetFreeSize() {
				t.Logf("totalUsed:%d, used:%d", totalMalloc-totalFree, allocator.Size-allocator.GetFreeSize())
				t.FailNow()
			}
		}()
		if free {
			if len(used) == 0 {
				return
			}
			for _, u := range used {
				if u[1] > size {
					totalFree += size
					t.Logf("totalFree1:%d, free:%v", totalFree, size)
					allocator.Free(u[0], size)
					u[1] -= size
					u[0] += size
					return
				}
			}
			allocator.Free(used[0][0], used[0][1])
			totalFree += used[0][1]
			t.Logf("totalFree2:%d, free:%v", totalFree, used[0][1])
			used = slices.Delete(used, 0, 1)
			return
		}
		offset := allocator.Allocate(size)
		if offset == -1 {
			return
		}
		used = append(used, [2]int{offset, size})
		totalMalloc += size
		t.Logf("totalMalloc:%d, free:%v", totalMalloc, size)
	})
}
