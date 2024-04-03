package util

import (
	"unsafe"
)

type Block [2]int

type MemoryAllocator struct {
	start  int64
	memory []byte
	Size   int
	blocks *List[Block]
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

func (ma *MemoryAllocator) Malloc2(size int) (memory []byte, start, end int) {
	for be := ma.blocks.Front(); be != nil; be = be.Next() {
		start, end = be.Value[0], be.Value[1]
		if e := start + size; end >= e {
			memory = ma.memory[start:e]
			if be.Value[0] = e; end == e {
				ma.blocks.Remove(be)
			}
			end = e
			return
		}
	}
	return
}

func (ma *MemoryAllocator) Malloc(size int) (memory []byte) {
	memory, _, _ = ma.Malloc2(size)
	return
}

func (ma *MemoryAllocator) Make(size int) (memory []byte) {
	memory = ma.Malloc(size)
	if memory == nil {
		return make([]byte, size)
	}
	return
}

func (ma *MemoryAllocator) Free2(start, end int) {
	if start < 0 || end > ma.Size {
		return
	}
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		block := e.Value
		if block[1] == start {
			block[1] = end
			return
		}
		if block[0] == end {
			block[0] = start
			return
		}
		if end > block[0] {
			ma.blocks.InsertBefore(Block{start, end}, e)
			return
		}
	}
	ma.blocks.PushBack(Block{start, end})
}

func (ma *MemoryAllocator) Free(mem []byte) {
	ptr := uintptr(unsafe.Pointer(&mem[:1][0]))
	start := int(int64(ptr) - ma.start)
	end := start + len(mem)
	ma.Free2(start, end)
}

type RecyclableMemory struct {
	*MemoryAllocator
	mem []int
}

func (r *RecyclableMemory) Malloc(size int) (memory []byte) {
	ret, start, end := r.Malloc2(size)
	if ret == nil {
		return make([]byte, size)
	}
	if lastI := len(r.mem) - 1; lastI > 0 && r.mem[lastI] == start {
		r.mem[lastI] = end
	} else {
		r.mem = append(r.mem, start, end)
	}
	return ret
}

func (r *RecyclableMemory) Recycle() {
	for i := 0; i < len(r.mem); i += 2 {
		r.Free2(r.mem[i], r.mem[i+1])
	}
	r.mem = r.mem[:0]
}
