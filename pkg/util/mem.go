package util

import (
	"slices"
	"sync"
	"unsafe"
)

const MaxBlockSize = 4 * 1024 * 1024

var pools sync.Map
var EnableCheckSize bool = false

type MemoryAllocator struct {
	allocator *Allocator
	start     int64
	memory    []byte
	Size      int
}

func GetMemoryAllocator(size int) (ret *MemoryAllocator) {
	if value, ok := pools.Load(size); ok {
		ret = value.(*sync.Pool).Get().(*MemoryAllocator)
	} else {
		ret = NewMemoryAllocator(size)
	}
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
	ma.allocator = NewAllocator(ma.Size)
	size := ma.Size
	pool, _ := pools.LoadOrStore(size, &sync.Pool{
		New: func() any {
			return NewMemoryAllocator(size)
		},
	})
	pool.(*sync.Pool).Put(ma)
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
}

func NewScalableMemoryAllocator(size int) (ret *ScalableMemoryAllocator) {
	if value, ok := pools.Load(size); ok {
		ret = value.(*sync.Pool).Get().(*ScalableMemoryAllocator)
	} else {
		ret = &ScalableMemoryAllocator{children: []*MemoryAllocator{NewMemoryAllocator(size)}, size: size}
	}
	return
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
	child = NewMemoryAllocator(max(min(MaxBlockSize, child.Size*2), size))
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
	mallocIndexes []int
}

func (r *RecyclableMemory) NextN(size int) (memory []byte) {
	memory = r.ScalableMemoryAllocator.Malloc(size)
	r.mallocIndexes = append(r.mallocIndexes, len(r.Buffers))
	r.ReadFromBytes(memory)
	return
}

func (r *RecyclableMemory) AddRecycleBytes(b ...[]byte) {
	start := len(r.Buffers)
	for i := range b {
		r.mallocIndexes = append(r.mallocIndexes, start+i)
	}
	r.ReadFromBytes(b...)
}

func (r *RecyclableMemory) RemoveRecycleBytes(index int) (buf []byte) {
	if index < 0 {
		index = len(r.Buffers) + index
	}
	buf = r.Buffers[index]
	i := slices.Index(r.mallocIndexes, index)
	r.mallocIndexes = slices.Delete(r.mallocIndexes, i, i+1)
	r.Buffers = slices.Delete(r.Buffers, index, index+1)
	r.Size -= len(buf)
	return
}

func (r *RecyclableMemory) Recycle() {
	for _, index := range r.mallocIndexes {
		r.Free(r.Buffers[index])
	}
}
