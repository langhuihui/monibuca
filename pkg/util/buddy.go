package util

import (
	"errors"
)

type Buddy struct {
	size     int
	longests []int
}

var (
	InValidParameterErr = errors.New("buddy: invalid parameter")
	NotFoundErr         = errors.New("buddy: can't find block")
)

// NewBuddy creates a buddy instance.
// If the parameter isn't valid, return the nil and error as well
func NewBuddy(size int) *Buddy {
	if !isPowerOf2(size) {
		size = fixSize(size)
	}
	nodeCount := 2*size - 1
	longests := make([]int, nodeCount)
	for nodeSize, i := 2*size, 0; i < nodeCount; i++ {
		if isPowerOf2(i + 1) {
			nodeSize /= 2
		}
		longests[i] = nodeSize
	}
	return &Buddy{size, longests}
}

// Alloc find a unused block according to the size
// return the offset of the block(regard 0 as the beginning)
// and parameter error if any
func (b *Buddy) Alloc(size int) (offset int, err error) {
	if size <= 0 {
		err = InValidParameterErr
		return
	}
	if !isPowerOf2(size) {
		size = fixSize(size)
	}
	if size > b.longests[0] {
		err = NotFoundErr
		return
	}
	index := 0
	for nodeSize := b.size; nodeSize != size; nodeSize /= 2 {
		if left := leftChild(index); b.longests[left] >= size {
			index = left
		} else {
			index = rightChild(index)
		}
	}
	b.longests[index] = 0 // mark zero as used
	offset = (index+1)*size - b.size
	// update the parent node's size
	for index != 0 {
		index = parent(index)
		b.longests[index] = max(b.longests[leftChild(index)], b.longests[rightChild(index)])
	}
	return
}

// Free find a block according to the offset and mark it as unused
// return error if not found or parameter invalid
func (b *Buddy) Free(offset int) error {
	if offset < 0 || offset >= b.size {
		return InValidParameterErr
	}
	nodeSize := 1
	index := offset + b.size - 1
	for ; b.longests[index] != 0; index = parent(index) {
		nodeSize *= 2
		if index == 0 {
			return NotFoundErr
		}
	}
	b.longests[index] = nodeSize
	// update parent node's size
	for index != 0 {
		index = parent(index)
		nodeSize *= 2

		leftSize := b.longests[leftChild(index)]
		rightSize := b.longests[rightChild(index)]
		if leftSize+rightSize == nodeSize {
			b.longests[index] = nodeSize
		} else {
			b.longests[index] = max(leftSize, rightSize)
		}
	}
	return nil
}

// helpers
func isPowerOf2(size int) bool {
	return size&(size-1) == 0
}

func fixSize(size int) int {
	size |= size >> 1
	size |= size >> 2
	size |= size >> 4
	size |= size >> 8
	size |= size >> 16
	return size + 1
}

func leftChild(index int) int {
	return 2*index + 1
}

func rightChild(index int) int {
	return 2*index + 2
}

func parent(index int) int {
	return (index+1)/2 - 1
}
