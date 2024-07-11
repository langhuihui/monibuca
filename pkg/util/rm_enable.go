//go:build !disable_rm

package util

type RecyclableMemory struct {
	allocator *ScalableMemoryAllocator
	Memory
	recycleIndexes []int
}

func (r *RecyclableMemory) InitRecycleIndexes(max int) {
	r.recycleIndexes = make([]int, 0, max)
}

func (r *RecyclableMemory) GetAllocator() *ScalableMemoryAllocator {
	return r.allocator
}

func (r *RecyclableMemory) NextN(size int) (memory []byte) {
	memory = r.allocator.Malloc(size)
	if r.recycleIndexes != nil {
		r.recycleIndexes = append(r.recycleIndexes, r.Count())
	}
	r.AppendOne(memory)
	return
}

func (r *RecyclableMemory) AddRecycleBytes(b []byte) {
	if r.recycleIndexes != nil {
		r.recycleIndexes = append(r.recycleIndexes, r.Count())
	}
	r.AppendOne(b)
}

func (r *RecyclableMemory) SetAllocator(allocator *ScalableMemoryAllocator) {
	r.allocator = allocator
}

func (r *RecyclableMemory) Recycle() {
	if r.recycleIndexes != nil {
		for _, index := range r.recycleIndexes {
			r.allocator.Free(r.Buffers[index])
		}
		r.recycleIndexes = r.recycleIndexes[:0]
	} else {
		for _, buf := range r.Buffers {
			r.allocator.Free(buf)
		}
	}
}
