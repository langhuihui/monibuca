//go:build disable_rm

package util

import (
	"io"
	"slices"
)

type RecyclableMemory struct {
	Memory
}

func NewRecyclableMemory(allocator *ScalableMemoryAllocator) RecyclableMemory {
	return RecyclableMemory{}
}

func (r *RecyclableMemory) Clone() RecyclableMemory {
	return RecyclableMemory{
		Memory: Memory{
			Buffers: slices.Clone(r.Buffers),
			Size:    r.Size,
		},
	}
}

func (r *RecyclableMemory) InitRecycleIndexes(max int) {
}

func (r *RecyclableMemory) GetAllocator() *ScalableMemoryAllocator {
	return nil
}

func (r *RecyclableMemory) SetAllocator(allocator *ScalableMemoryAllocator) {
}

func (r *RecyclableMemory) Recycle() {
}

func (r *RecyclableMemory) NextN(size int) (memory []byte) {
	memory = make([]byte, size)
	r.AppendOne(memory)
	return memory
}

func (r *RecyclableMemory) AddRecycleBytes(b []byte) {
	r.AppendOne(b)
}

type MemoryAllocator struct {
	Size int
}

func (*MemoryAllocator) GetBlocks() (blocks []*Block) {
	return nil
}

type ScalableMemoryAllocator struct {
}

func NewScalableMemoryAllocator(size int) (ret *ScalableMemoryAllocator) {
	return nil
}

func (*ScalableMemoryAllocator) Malloc(size int) (memory []byte) {
	return make([]byte, size)
}

func (*ScalableMemoryAllocator) FreeRest(mem *[]byte, keep int) {
	if m := *mem; keep < len(m) {
		*mem = m[:keep]
	}
}

func (*ScalableMemoryAllocator) GetChildren() []*MemoryAllocator {
	return nil
}

func (*ScalableMemoryAllocator) Read(reader io.Reader, n int) (mem []byte, err error) {
	mem = make([]byte, n)
	n, err = reader.Read(mem)
	return mem[:n], err
}

func (*ScalableMemoryAllocator) Borrow(size int) (memory []byte) {
	return make([]byte, size)
}

func (*ScalableMemoryAllocator) Recycle() {
}

func (*ScalableMemoryAllocator) Free(mem []byte) bool {
	return true
}
