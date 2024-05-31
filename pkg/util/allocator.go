package util

type (
	Tree struct {
		Left, Right *Block
		Height      int
	}
	Block struct {
		Start, End int
		trees      [2]Tree
		allocator  *Allocator
	}
	// History struct {
	// 	Malloc bool
	// 	Offset int
	// 	Size   int
	// }
	Allocator struct {
		pool       []*Block
		SizeTree   *Block
		OffsetTree *Block
		Size       int
		// history    []History
	}
)

func (t *Tree) deleteLeft(b *Block, treeIndex int) {
	t.Left = t.Left.delete(b, treeIndex)
}

func (t *Tree) deleteRight(b *Block, treeIndex int) {
	t.Right = t.Right.delete(b, treeIndex)
}

func NewAllocator(size int) (result *Allocator) {
	root := &Block{Start: 0, End: size}
	result = &Allocator{
		SizeTree:   root,
		OffsetTree: root,
		Size:       size,
	}
	root.allocator = result
	return
}
func compareBySize(a, b *Block) bool {
	if sizea, sizeb := a.End-a.Start, b.End-b.Start; sizea != sizeb {
		return sizea < sizeb
	}
	return a.Start < b.Start
}

func compareByOffset(a, b *Block) bool {
	return a.Start < b.Start
}

var compares = [...]func(a, b *Block) bool{compareBySize, compareByOffset}
var emptyTrees = [2]Tree{}

func (b *Block) recycle() {
	b.allocator.putBlock(b)
}

func (b *Block) insert(block *Block, treeIndex int) *Block {
	if b == nil {
		return block
	}
	if tree := &b.trees[treeIndex]; compares[treeIndex](block, b) {
		tree.Left = tree.Left.insert(block, treeIndex)
	} else {
		tree.Right = tree.Right.insert(block, treeIndex)
	}
	b.updateHeight(treeIndex)
	return b.balance(treeIndex)
}

func (b *Block) getLeftHeight(treeIndex int) int {
	return b.trees[treeIndex].Left.getHeight(treeIndex)
}

func (b *Block) getRightHeight(treeIndex int) int {
	return b.trees[treeIndex].Right.getHeight(treeIndex)
}

func (b *Block) getHeight(treeIndex int) int {
	if b == nil {
		return 0
	}
	return b.trees[treeIndex].Height
}

func (b *Block) updateHeight(treeIndex int) {
	b.trees[treeIndex].Height = 1 + max(b.getLeftHeight(treeIndex), b.getRightHeight(treeIndex))
}

func (b *Block) balance(treeIndex int) *Block {
	if b == nil {
		return nil
	}
	if tree := &b.trees[treeIndex]; b.getLeftHeight(treeIndex)-b.getRightHeight(treeIndex) > 1 {
		if tree.Left.getRightHeight(treeIndex) > tree.Left.getLeftHeight(treeIndex) {
			tree.Left = tree.Left.rotateLeft(treeIndex)
		}
		return b.rotateRight(treeIndex)
	} else if b.getRightHeight(treeIndex)-b.getLeftHeight(treeIndex) > 1 {
		if tree.Right.getLeftHeight(treeIndex) > tree.Right.getRightHeight(treeIndex) {
			tree.Right = tree.Right.rotateRight(treeIndex)
		}
		return b.rotateLeft(treeIndex)
	}
	return b
}

func (b *Block) rotateLeft(treeIndex int) *Block {
	newRoot := b.trees[treeIndex].Right
	b.trees[treeIndex].Right = newRoot.trees[treeIndex].Left
	newRoot.trees[treeIndex].Left = b
	b.updateHeight(treeIndex)
	newRoot.updateHeight(treeIndex)
	return newRoot
}

func (b *Block) rotateRight(treeIndex int) *Block {
	newRoot := b.trees[treeIndex].Left
	b.trees[treeIndex].Left = newRoot.trees[treeIndex].Right
	newRoot.trees[treeIndex].Right = b
	b.updateHeight(treeIndex)
	newRoot.updateHeight(treeIndex)
	return newRoot
}

func (b *Block) findMin(treeIndex int) *Block {
	if left := b.trees[treeIndex].Left; left == nil {
		return b
	} else {
		return left.findMin(treeIndex)
	}
}

func (b *Block) delete(block *Block, treeIndex int) *Block {
	if b == nil {
		return nil
	}
	if compareFunc, tree := compares[treeIndex], &b.trees[treeIndex]; b == block {
		if tree.Left == nil {
			return tree.Right
		} else if tree.Right == nil {
			return tree.Left
		}
		minBlock := tree.Right.findMin(treeIndex)
		tree.deleteRight(minBlock, treeIndex)
		minTree := &minBlock.trees[treeIndex]
		minTree.Left = tree.Left
		minTree.Right = tree.Right
		minTree.Height = tree.Height
		return minBlock
	} else if compareFunc(block, b) {
		tree.deleteLeft(block, treeIndex)
	} else {
		tree.deleteRight(block, treeIndex)
	}
	b.updateHeight(treeIndex)
	return b.balance(treeIndex)
}

func (a *Allocator) Allocate(size int) (offset int) {
	// a.history = append(a.history, History{Malloc: true, Size: size})
	block := a.findAvailableBlock(a.SizeTree, size)
	if block == nil {
		return -1
	}
	offset = block.Start
	a.deleteSizeTree(block)
	a.deleteOffsetTree(block)
	if newStart := offset + size; newStart < block.End {
		newBlock := a.getBlock(newStart, block.End)
		a.SizeTree = a.SizeTree.insert(newBlock, 0)
		a.OffsetTree = a.OffsetTree.insert(newBlock, 1)
		// block.End = block.Start + size
	}
	return
}

func (a *Allocator) findAvailableBlock(block *Block, size int) *Block {
	if block == nil {
		return nil
	}
	if tree := &block.trees[0]; block.End-block.Start < size {
		if block1 := a.findAvailableBlock(tree.Left, size); block1 == nil {
			return a.findAvailableBlock(tree.Right, size)
		} else {
			return block1
		}
	}
	return block
}

func (a *Allocator) getBlock(start, end int) *Block {
	if l := len(a.pool); l == 0 {
		return &Block{Start: start, End: end, allocator: a}
	} else {
		block := a.pool[l-1]
		a.pool = a.pool[:l-1]
		block.Start, block.End = start, end
		block.allocator = a
		return block
	}
}

func (a *Allocator) putBlock(b *Block) {
	if b.allocator == nil {
		return
	}
	b.trees = emptyTrees
	b.allocator = nil
	a.pool = append(a.pool, b)
}

func (a *Allocator) Free(offset, size int) {
	// a.history = append(a.history, History{Malloc: false, Offset: offset, Size: size})
	block := a.getBlock(offset, offset+size)
	a.SizeTree, a.OffsetTree = a.SizeTree.insert(block, 0), a.OffsetTree.insert(block, 1)
	a.mergeAdjacentBlocks(block)
}

func (a *Allocator) GetBlocks() (blocks []*Block) {
	a.OffsetTree.Walk(func(block *Block) {
		blocks = append(blocks, block)
	}, 1)
	return
}

func (a *Allocator) GetFreeSize() (ret int) {
	a.OffsetTree.Walk(func(block *Block) {
		ret += block.End - block.Start
	}, 1)
	return
}

func (a *Allocator) deleteSizeTree(block *Block) {
	a.SizeTree = a.SizeTree.delete(block, 0)
	block.trees[0] = emptyTrees[0]
}

func (a *Allocator) deleteOffsetTree(block *Block) {
	a.OffsetTree = a.OffsetTree.delete(block, 1)
	block.trees[1] = emptyTrees[1]
}

func (a *Allocator) mergeAdjacentBlocks(block *Block) {
	if leftAdjacent := a.OffsetTree.findLeftAdjacentBlock(block.Start); leftAdjacent != nil {
		a.deleteSizeTree(leftAdjacent)
		a.deleteOffsetTree(leftAdjacent)
		a.deleteSizeTree(block)
		block.Start = leftAdjacent.Start
		a.SizeTree = a.SizeTree.insert(block, 0)
		leftAdjacent.recycle()
	}
	if rightAdjacent := a.OffsetTree.findRightAdjacentBlock(block.End); rightAdjacent != nil {
		a.deleteSizeTree(rightAdjacent)
		a.deleteOffsetTree(rightAdjacent)
		a.deleteSizeTree(block)
		block.End = rightAdjacent.End
		a.SizeTree = a.SizeTree.insert(block, 0)
		rightAdjacent.recycle()
	}
}

func (b *Block) findLeftAdjacentBlock(offset int) *Block {
	if b == nil {
		return nil
	}
	tree := &b.trees[1]
	if b.End == offset {
		return b
	}
	if b.End > offset {
		return tree.Left.findLeftAdjacentBlock(offset)
	}
	return tree.Right.findLeftAdjacentBlock(offset)
}

func (b *Block) findRightAdjacentBlock(offset int) *Block {
	if b == nil {
		return nil
	}
	if b.Start == offset {
		return b
	}
	tree := &b.trees[1]
	if b.Start < offset {
		return tree.Right.findRightAdjacentBlock(offset)
	}
	return tree.Left.findRightAdjacentBlock(offset)
}

func (b *Block) Walk(fn func(*Block), index int) {
	if b == nil {
		return
	}
	b.trees[index].Left.Walk(fn, index)
	fn(b)
	b.trees[index].Right.Walk(fn, index)
}
