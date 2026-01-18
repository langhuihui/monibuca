package plugin_gb28181pro

import (
	"math/bits"
	"sync/atomic"
)

// PortBitmap 使用原子位图实现端口分配/回收
type PortBitmap struct {
	base   uint16
	size   uint16
	bitmap []uint64
	cursor uint32
}

func (pb *PortBitmap) Init(base uint16, size uint16) {
	pb.base = base
	pb.size = size
	words := int((uint32(size) + 63) / 64)
	pb.bitmap = make([]uint64, words)
	atomic.StoreUint32(&pb.cursor, 0)
}

func (pb *PortBitmap) Allocate() (uint16, bool) {
	if pb.size == 0 || len(pb.bitmap) == 0 {
		return 0, false
	}
	words := len(pb.bitmap)
	start := int(atomic.LoadUint32(&pb.cursor) % uint32(words))
	for i := 0; i < words; i++ {
		widx := (start + i) % words
		for {
			old := atomic.LoadUint64(&pb.bitmap[widx])
			free := ^old
			if free == 0 {
				break
			}
			pick := free & -free
			newv := old | pick
			if atomic.CompareAndSwapUint64(&pb.bitmap[widx], old, newv) {
				bit := uint64(bits.TrailingZeros64(pick))
				idx := uint64(widx)*64 + bit
				if idx >= uint64(pb.size) {
					// 回滚越界位
					for {
						cur := atomic.LoadUint64(&pb.bitmap[widx])
						reverted := cur &^ pick
						if atomic.CompareAndSwapUint64(&pb.bitmap[widx], cur, reverted) {
							break
						}
					}
					break
				}
				atomic.StoreUint32(&pb.cursor, uint32(widx))
				return pb.base + uint16(idx), true
			}
		}
	}
	return 0, false
}

func (pb *PortBitmap) Release(port uint16) bool {
	if pb.size == 0 || len(pb.bitmap) == 0 {
		return false
	}
	if port < pb.base {
		return false
	}
	idx := uint32(port - pb.base)
	if idx >= uint32(pb.size) {
		return false
	}
	widx := idx / 64
	bit := idx % 64
	mask := uint64(1) << bit
	for {
		old := atomic.LoadUint64(&pb.bitmap[widx])
		if old&mask == 0 {
			return false
		}
		newv := old &^ mask
		if atomic.CompareAndSwapUint64(&pb.bitmap[widx], old, newv) {
			return true
		}
	}
}

// GetAvailablePorts 返回当前可用的端口数量
func (pb *PortBitmap) GetAvailablePorts() int {
	if pb.size == 0 || len(pb.bitmap) == 0 {
		return 0
	}
	used := 0
	for i := 0; i < len(pb.bitmap); i++ {
		used += bits.OnesCount64(atomic.LoadUint64(&pb.bitmap[i]))
	}
	return int(pb.size) - used
}

// GetUsedPorts 返回当前已使用的端口数量
func (pb *PortBitmap) GetUsedPorts() int {
	if pb.size == 0 || len(pb.bitmap) == 0 {
		return 0
	}
	used := 0
	for i := 0; i < len(pb.bitmap); i++ {
		used += bits.OnesCount64(atomic.LoadUint64(&pb.bitmap[i]))
	}
	return used
}

// GetUsedPortList 返回当前被占用的端口列表
func (pb *PortBitmap) GetUsedPortList() []uint16 {
	if pb.size == 0 || len(pb.bitmap) == 0 {
		return nil
	}
	var usedPorts []uint16
	for i := 0; i < len(pb.bitmap); i++ {
		bitmap := atomic.LoadUint64(&pb.bitmap[i])
		if bitmap != 0 {
			for j := 0; j < 64; j++ {
				if bitmap&(1<<j) != 0 {
					port := pb.base + uint16(i*64+j)
					if port < pb.base+pb.size {
						usedPorts = append(usedPorts, port)
					}
				}
			}
		}
	}
	return usedPorts
}


