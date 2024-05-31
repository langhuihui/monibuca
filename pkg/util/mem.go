package util

import (
	"fmt"
	"unsafe"
)

type MemoryAllocator struct {
	allocator *Allocator
	start     int64
	memory    []byte
	Size      int
}

func NewMemoryAllocator(size int) (ret *MemoryAllocator) {
	ret = &MemoryAllocator{
		Size:      size,
		memory:    make([]byte, size),
		allocator: NewAllocator(size),
	}
	ret.start = int64(uintptr(unsafe.Pointer(&ret.memory[0])))
	return
}

func (ma *MemoryAllocator) Malloc(size int) (memory []byte) {
	if offset := ma.allocator.Allocate(size); offset != -1 {
		memory = ma.memory[offset : offset+size]
	}
	return
}

func (ma *MemoryAllocator) free(start, size int) (ret bool) {
	if start < 0 || start+size > ma.Size {
		return
	}
	ma.allocator.Free(start, size)
	return true
}

func (ma *MemoryAllocator) Free(mem []byte) bool {
	start := int(int64(uintptr(unsafe.Pointer(&mem[0]))) - ma.start)
	return ma.free(start, len(mem))
}

func (ma *MemoryAllocator) GetBlocks() (blocks []*Block) {
	return ma.allocator.GetBlocks()
}

var EnableCheckSize bool = false

type ScalableMemoryAllocator struct {
	children    []*MemoryAllocator
	totalMalloc int64
	totalFree   int64
	size        int
}

func NewScalableMemoryAllocator(size int) (ret *ScalableMemoryAllocator) {
	return &ScalableMemoryAllocator{children: []*MemoryAllocator{NewMemoryAllocator(size)}, size: size}
}

func (sma *ScalableMemoryAllocator) checkSize() {
	var totalFree int
	for _, child := range sma.children {
		totalFree += child.allocator.GetFreeSize()
	}
	if totalFree != sma.size-(int(sma.totalMalloc)-int(sma.totalFree)) {
		panic("CheckSize")
	}
}

func (sma *ScalableMemoryAllocator) addMallocCount(size int) {
	sma.totalMalloc += int64(size)
}

func (sma *ScalableMemoryAllocator) addFreeCount(size int) {
	sma.totalFree += int64(size)
}

func (sma *ScalableMemoryAllocator) GetTotalMalloc() int64 {
	return sma.totalMalloc
}

func (sma *ScalableMemoryAllocator) GetTotalFree() int64 {
	return sma.totalFree
}

func (sma *ScalableMemoryAllocator) GetChildren() []*MemoryAllocator {
	return sma.children
}

func (sma *ScalableMemoryAllocator) Malloc(size int) (memory []byte) {
	if sma == nil {
		return make([]byte, size)
	}
	if EnableCheckSize {
		defer sma.checkSize()
	}
	defer sma.addMallocCount(size)
	var child *MemoryAllocator
	for _, child = range sma.children {
		if memory = child.Malloc(size); memory != nil {
			return
		}
	}
	child = NewMemoryAllocator(max(child.Size*2, size))
	sma.size += child.Size
	memory = child.Malloc(size)
	sma.children = append(sma.children, child)
	return
}

func (sma *ScalableMemoryAllocator) GetScalableMemoryAllocator() *ScalableMemoryAllocator {
	return sma
}

func (sma *ScalableMemoryAllocator) Free(mem []byte) bool {
	if sma == nil {
		return false
	}
	if EnableCheckSize {
		defer sma.checkSize()
	}
	ptr := int64(uintptr(unsafe.Pointer(&mem[0])))
	size := len(mem)
	for _, child := range sma.children {
		if start := int(ptr - child.start); start >= 0 && start < child.Size && child.free(start, size) {
			sma.addFreeCount(size)
			return true
		}
	}
	return false
}

type RecyclableMemory struct {
	*ScalableMemoryAllocator
	Memory
}

func (r *RecyclableMemory) NextN(size int) (memory []byte) {
	memory = r.ScalableMemoryAllocator.Malloc(size)
	r.Memory.ReadFromBytes(memory)
	return
}

func (r *RecyclableMemory) Recycle() {
	for i, buf := range r.Memory.Buffers {
		ret := r.Free(buf)
		if !ret {
			fmt.Println(i)
		}
	}
}
