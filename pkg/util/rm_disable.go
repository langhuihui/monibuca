//go:build disable_rm

package util

type RecyclableMemory struct {
	Memory
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
