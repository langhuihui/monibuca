package util

import (
	"slices"
	"sync"
	"unsafe"
)

const (
	MaxBlockSize = 1 << 22
	BuddySize    = MaxBlockSize << 4
	MinPowerOf2  = 10
)

var (
	memoryPool [BuddySize]byte
	buddy      = NewBuddy(BuddySize >> MinPowerOf2)
	lock       sync.Mutex
	poolStart  = int64(uintptr(unsafe.Pointer(&memoryPool[0])))
	//EnableCheckSize bool = false
)

type MemoryAllocator struct {
	allocator *Allocator
	start     int64
	memory    []byte
	Size      int
}

func GetMemoryAllocator(size int) (ret *MemoryAllocator) {
	lock.Lock()
	offset, err := buddy.Alloc(size >> MinPowerOf2)
	lock.Unlock()
	if err != nil {
		return NewMemoryAllocator(size)
	}
	offset = offset << MinPowerOf2
	ret = &MemoryAllocator{
		Size:      size,
		memory:    memoryPool[offset : offset+size],
		allocator: NewAllocator(size),
	}
	ret.start = poolStart + int64(offset)
	return
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

func (ma *MemoryAllocator) Recycle() {
	lock.Lock()
	_ = buddy.Free(int((poolStart - ma.start) >> 10))
	lock.Unlock()
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

type ScalableMemoryAllocator struct {
	children    []*MemoryAllocator
	totalMalloc int64
	totalFree   int64
	size        int
	childSize   int
}

func NewScalableMemoryAllocator(size int) (ret *ScalableMemoryAllocator) {
	return &ScalableMemoryAllocator{children: []*MemoryAllocator{GetMemoryAllocator(size)}, size: size, childSize: size}
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

func (sma *ScalableMemoryAllocator) Recycle() {
	for _, child := range sma.children {
		child.Recycle()
	}
}

func (sma *ScalableMemoryAllocator) Malloc(size int) (memory []byte) {
	if sma == nil {
		return make([]byte, size)
	}
	//if EnableCheckSize {
	//	defer sma.checkSize()
	//}
	defer sma.addMallocCount(size)
	var child *MemoryAllocator
	for _, child = range sma.children {
		if memory = child.Malloc(size); memory != nil {
			return
		}
	}
	for sma.childSize <= MaxBlockSize {
		sma.childSize = sma.childSize << 1
		if sma.childSize >= size {
			break
		}
	}
	child = GetMemoryAllocator(sma.childSize)
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
	//if EnableCheckSize {
	//	defer sma.checkSize()
	//}
	ptr := int64(uintptr(unsafe.Pointer(&mem[0])))
	size := len(mem)
	for i, child := range sma.children {
		if start := int(ptr - child.start); start >= 0 && start < child.Size && child.free(start, size) {
			sma.addFreeCount(size)
			if len(sma.children) > 1 && child.allocator.sizeTree.End-child.allocator.sizeTree.Start == child.Size {
				child.Recycle()
				sma.children = slices.Delete(sma.children, i, i+1)
				sma.size -= child.Size
			}
			return true
		}
	}
	return false
}

type RecyclableMemory struct {
	*ScalableMemoryAllocator
	Memory
	RecycleIndexes []int
}

func (r *RecyclableMemory) NextN(size int) (memory []byte) {
	memory = r.ScalableMemoryAllocator.Malloc(size)
	if r.RecycleIndexes != nil {
		r.RecycleIndexes = append(r.RecycleIndexes, len(r.Buffers))
	}
	r.ReadFromBytes(memory)
	return
}

func (r *RecyclableMemory) AddRecycleBytes(b ...[]byte) {
	if r.RecycleIndexes != nil {
		start := len(r.Buffers)
		for i := range b {
			r.RecycleIndexes = append(r.RecycleIndexes, start+i)
		}
	}
	r.ReadFromBytes(b...)
}

func (r *RecyclableMemory) RemoveRecycleBytes(index int) (buf []byte) {
	if index < 0 {
		index = len(r.Buffers) + index
	}
	buf = r.Buffers[index]
	if r.RecycleIndexes != nil {
		i := slices.Index(r.RecycleIndexes, index)
		r.RecycleIndexes = slices.Delete(r.RecycleIndexes, i, i+1)
	}
	r.Buffers = slices.Delete(r.Buffers, index, index+1)
	r.Size -= len(buf)
	return
}

func (r *RecyclableMemory) Recycle() {
	if r.RecycleIndexes != nil {
		for _, index := range r.RecycleIndexes {
			r.Free(r.Buffers[index])
		}
		r.RecycleIndexes = r.RecycleIndexes[:0]
	} else {
		for _, buf := range r.Buffers {
			r.Free(buf)
		}
	}
}
