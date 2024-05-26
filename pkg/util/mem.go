package util

import (
	"unsafe"
)

type Block [2]int

func (block Block) Len() int {
	return block[1] - block[0]
}

func (block Block) Split() (int, int) {
	return block[0], block[1]
}

func (block *Block) Combine(s, e int) (ret bool) {
	if ret = block[0] == e; ret {
		block[0] = s
	} else if ret = block[1] == s; ret {
		block[1] = e
	}
	return
}

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
		start, end = be.Value.Split()
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

func (ma *MemoryAllocator) Free2(start, end int) bool {
	if start < 0 || end > ma.Size || start >= end {
		return false
	}
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		if e.Value.Combine(start, end) {
			return true
		}
		if end < e.Value[0] {
			ma.blocks.InsertBefore(Block{start, end}, e)
			return true
		}
	}
	ma.blocks.PushBack(Block{start, end})
	return true
}

func (ma *MemoryAllocator) Free(mem []byte) bool {
	ptr := uintptr(unsafe.Pointer(&mem[:1][0]))
	start := int(int64(ptr) - ma.start)
	return ma.Free2(start, start+len(mem))
}

func (ma *MemoryAllocator) GetBlocks() (blocks []Block) {
	for e := ma.blocks.Front(); e != nil; e = e.Next() {
		blocks = append(blocks, e.Value)
	}
	return
}

type ScalableMemoryAllocator []*MemoryAllocator

func NewScalableMemoryAllocator(size int) (ret *ScalableMemoryAllocator) {
	return &ScalableMemoryAllocator{NewMemoryAllocator(size)}
}

func (sma *ScalableMemoryAllocator) Malloc(size int) (memory []byte) {
	if sma == nil {
		return make([]byte, size)
	}
	memory, _, _, _ = sma.Malloc2(size)
	return memory
}

func (sma *ScalableMemoryAllocator) Malloc2(size int) (memory []byte, index, start, end int) {
	for i, child := range *sma {
		index = i
		if memory, start, end = child.Malloc2(size); memory != nil {
			return
		}
	}
	n := NewMemoryAllocator(max((*sma)[index].Size*2, size))
	index++
	memory, start, end = n.Malloc2(size)
	*sma = append(*sma, n)
	return
}
func (sma *ScalableMemoryAllocator) GetScalableMemoryAllocator() *ScalableMemoryAllocator {
	return sma
}
func (sma *ScalableMemoryAllocator) Free(mem []byte) bool {
	if sma == nil {
		return false
	}
	ptr := uintptr(unsafe.Pointer(&mem[:1][0]))
	for _, child := range *sma {
		if start := int(int64(ptr) - child.start); child.Free2(start, start+len(mem)) {
			return true
		}
	}
	return false
}

func (sma *ScalableMemoryAllocator) Free2(index, start, end int) bool {
	if index < 0 || index >= len(*sma) {
		return false
	}
	return (*sma)[index].Free2(start, end)
}

// type RecyclableMemory struct {
// 	*ScalableMemoryAllocator
// 	mem []int
// }

// func (r *RecyclableMemory) Malloc(size int) (memory []byte) {
// 	ret, i, start, end := r.Malloc2(size)
// 	// ml := len(r.mem)
// 	// if lastI, lastE := ml-3, ml-1; lastI > 0 && r.mem[lastI] == i && r.mem[lastE] == start {
// 	// 	r.mem[lastE] = end
// 	// } else {
// 	r.mem = append(r.mem, i, start, end)
// 	// }
// 	return ret
// }

// func (r *RecyclableMemory) Pop() []int {
// 	l := len(r.mem)
// 	if l == 0 {
// 		return nil
// 	}
// 	ret := r.mem[l-3:]
// 	r.mem = r.mem[:l-3]
// 	return ret
// }

// func (r *RecyclableMemory) Push(args ...int) {
// 	r.mem = append(r.mem, args...)
// }

// func (r *RecyclableMemory) Recycle() {
// 	for i := 0; i < len(r.mem); i += 3 {
// 		r.Free2(r.mem[i], r.mem[i+1], r.mem[i+2])
// 	}
// 	r.mem = r.mem[:0]
// }

// func (r *RecyclableMemory) RecycleBack(n int) {
// 	l := len(r.mem)
// 	end := &r.mem[l-1]
// 	start := *end - n
// 	r.Free2(r.mem[l-3], start, *end)
// 	*end = start
// 	if start == r.mem[l-2] {
// 		r.mem = r.mem[:l-3]
// 	}
// }

type RecyclableBuffers struct {
	*ScalableMemoryAllocator
	Buffers
}

func (r *RecyclableBuffers) NextN(size int) (memory []byte) {
	memory = r.ScalableMemoryAllocator.Malloc(size)
	r.Buffers.ReadFromBytes(memory)
	return
}

func (r *RecyclableBuffers) Recycle() {
	for _, buf := range r.Buffers.Buffers {
		r.Free(buf)
	}
}

func (r *RecyclableBuffers) RecycleBack(n int) {
	r.Free(r.ClipBack(n))
}

func (r *RecyclableBuffers) RecycleFront() {
	for _, buf := range r.Buffers.ClipFront() {
		r.Free(buf)
	}
}

// func (r *RecyclableBuffers) Cut(n int) (child RecyclableBuffers) {
// 	child.ScalableMemoryAllocator = r.ScalableMemoryAllocator
// 	child.ReadFromBytes(r.Buffers.Cut(n)...)
// 	return
// }

type IAllocator interface {
	Malloc(int) []byte
	Free([]byte) bool
}
