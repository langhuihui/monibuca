package rtp

var RTPReorderBufferLen uint16 = 50

// RTPReorder RTP包乱序重排
type RTPReorder[T comparable] struct {
	lastSeq uint16 //最新收到的rtp包序号
	queue   []T    // 缓存队列,0号元素位置代表lastReq+1，永远保持为空
	Total   uint32 // 总共收到的包数量
	Drop    uint32 // 丢弃的包数量
}

func (p *RTPReorder[T]) Push(seq uint16, v T) (result T) {
	p.Total++
	// 初始化
	if len(p.queue) == 0 {
		p.lastSeq = seq
		p.queue = make([]T, RTPReorderBufferLen)
		return v
	}
	if seq < p.lastSeq && p.lastSeq-seq < 0x8000 {
		// 旧的包直接丢弃
		p.Drop++
		return
	}
	delta := seq - p.lastSeq
	if delta == 0 {
		// 重复包
		p.Drop++
		return
	}
	if delta == 1 {
		// 正常顺序,无需缓存
		p.lastSeq = seq
		p.pop()
		return v
	}
	if RTPReorderBufferLen < delta {
		//超过缓存最大范围,无法挽回,只能造成丢包（序号断裂）
		for {
			p.lastSeq++
			delta--
			head := p.pop()
			// 可以放得进去了
			if delta == RTPReorderBufferLen {
				p.queue[RTPReorderBufferLen-1] = v
				p.queue[0] = result
				return head
			} else if head != result {
				p.Drop++
			}
		}
	} else {
		// 出现后面的包先到达，缓存起来
		p.queue[delta-1] = v
		return
	}
}

func (p *RTPReorder[T]) pop() (result T) {
	copy(p.queue, p.queue[1:]) //整体数据向前移动一位，保持0号元素代表lastSeq+1
	p.queue[RTPReorderBufferLen-1] = result
	return p.queue[0]
}

// Pop 从缓存中取出一个包，需要连续调用直到返回nil
func (p *RTPReorder[T]) Pop() (result T) {
	if len(p.queue) == 0 {
		return
	}
	if next := p.queue[0]; next != result {
		result = next
		p.lastSeq++
		p.pop()
	}
	return
}
