//go:build enable_buddy

package util

import "unsafe"

func createMemoryAllocator(size int, buddy *Buddy, offset int) *MemoryAllocator {
	ret := &MemoryAllocator{
		allocator: NewAllocator(size),
		Size:      size,
		memory:    buddy.memoryPool[offset : offset+size],
		start:     buddy.poolStart + int64(offset),
		recycle: func() {
			buddy.Free(offset >> MinPowerOf2)
		},
	}
	ret.allocator.Init(size)
	return ret
}

func GetMemoryAllocator(size int) (ret *MemoryAllocator) {
	if size < BuddySize {
		requiredSize := size >> MinPowerOf2
		// 循环尝试从池中获取可用的 buddy
		for {
			buddy := GetBuddy()
			defer PutBuddy(buddy)
			offset, err := buddy.Alloc(requiredSize)
			if err == nil {
				// 分配成功，使用这个 buddy
				return createMemoryAllocator(size, buddy, offset<<MinPowerOf2)
			}
		}
	}
	// 池中的 buddy 都无法分配或大小不够，使用系统内存
	memory := make([]byte, size)
	start := int64(uintptr(unsafe.Pointer(&memory[0])))
	return &MemoryAllocator{
		allocator: NewAllocator(size),
		Size:      size,
		memory:    memory,
		start:     start,
	}
}
