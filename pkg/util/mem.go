package util

import (
	"fmt"
	"unsafe"
)

type Block struct {
	start int
	end   int
}

func (block Block) Len() int {
	return block.end - block.start
}

func (block *Block) Combine(s, e int) (ret bool) {
	if block.start == e {
		block.start = s
	} else if block.end == s {
		block.end = e
	} else {
		return
	}
	return true
}

func (block *Block) CutFront(n int) bool {
	if n > block.Len() {
		return false
	}
	block.start += n
	return true
}

type MemoryAllocator struct {
	start  int64
	memory []byte
	blocks *List[Block]
	Size   int
}

func NewMemoryAllocator(size int) (ret *MemoryAllocator) {
	ret = &MemoryAllocator{
		Size:   size,
		memory: make([]byte, size),
		blocks: NewList[Block](),
	}
	ret.start = int64(uintptr(unsafe.Pointer(&ret.memory[0])))
	ret.blocks.PushBack(Block{0, size})
	return
}

func (ma *MemoryAllocator) Malloc(size int) (memory []byte) {
	for be := ma.blocks.Front(); be != nil; be = be.Next() {
		if start := be.Value.start; be.Value.CutFront(size) {
			if be.Value.Len() == 0 {
				ma.blocks.Remove(be)
			}
			memory = ma.memory[start:be.Value.start]
			return
		}
	}
	return
}

func (ma *MemoryAllocator) GetFreeSize() (ret int) {
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		ret += e.Value.Len()
	}
	return
}

func (ma *MemoryAllocator) free(start, end int) (ret bool) {
	if start < 0 || end > ma.Size || start >= end {
		return
	}
	ret = true
	l := end - start
	freeSize := ma.GetFreeSize()
	defer func() {
		if freeSize+l != ma.GetFreeSize() {
			panic("freeSize")
		}
	}()
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		if end < e.Value.start {
			ma.blocks.InsertBefore(Block{start, end}, e)
			return
		}
		// combine to next block
		if e.Value.start == end {
			e.Value.start = start
			return
		}
		// combine to previous block
		if e.Value.end == start {
			e.Value.end = end
			// combine 3 blocks
			if next := e.Next(); next != nil && next.Value.start == end {
				e.Value.end = next.Value.end
				ma.blocks.Remove(next)
			}
			return
		}
	}
	ma.blocks.PushBack(Block{start, end})
	return
}

func (ma *MemoryAllocator) Free(mem []byte) bool {
	ptr := uintptr(unsafe.Pointer(&mem[0]))
	start := int(int64(ptr) - ma.start)
	return ma.free(start, start+len(mem))
}

func (ma *MemoryAllocator) GetBlocks() (blocks [][2]int) {
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		blocks = append(blocks, [2]int{e.Value.start, e.Value.end})
	}
	return
}

var EnableCheckSize bool = true

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
		totalFree += child.GetFreeSize()
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
		if start := int(ptr - child.start); child.free(start, start+size) {
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
