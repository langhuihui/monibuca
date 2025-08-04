package pkg

import (
	"fmt"

	"m7s.live/v5/pkg/util"
)

// AnnexBReader 专门用于读取 AnnexB 格式数据的读取器
// 模仿 MemoryReader 结构，支持跨切片读取和动态数据管理
type AnnexBReader struct {
	util.Memory                  // 存储数据的多段内存
	Length, offset0, offset1 int // 可读长度和当前读取位置
}

// AppendBuffer 追加单个数据缓冲区
func (r *AnnexBReader) AppendBuffer(buf []byte) {
	r.PushOne(buf)
	r.Length += len(buf)
}

// ClipFront 剔除已读取的数据，释放内存
func (r *AnnexBReader) ClipFront() {
	readOffset := r.Size - r.Length
	if readOffset == 0 {
		return
	}

	// 剔除已完全读取的缓冲区（不回收内存）
	if r.offset0 > 0 {
		r.Buffers = r.Buffers[r.offset0:]
		r.Size -= readOffset
		r.offset0 = 0
	}

	// 处理部分读取的缓冲区（不回收内存）
	if r.offset1 > 0 && len(r.Buffers) > 0 {
		buf := r.Buffers[0]
		r.Buffers[0] = buf[r.offset1:]
		r.Size -= r.offset1
		r.offset1 = 0
	}
}

// FindStartCode 查找 NALU 起始码，返回起始码位置和长度
func (r *AnnexBReader) FindStartCode() (pos int, startCodeLen int, found bool) {
	if r.Length < 3 {
		return 0, 0, false
	}

	// 逐字节检查起始码
	for i := 0; i <= r.Length-3; i++ {
		// 优先检查 4 字节起始码
		if i <= r.Length-4 {
			if r.getByteAt(i) == 0x00 && r.getByteAt(i+1) == 0x00 &&
				r.getByteAt(i+2) == 0x00 && r.getByteAt(i+3) == 0x01 {
				return i, 4, true
			}
		}

		// 检查 3 字节起始码（但要确保不是 4 字节起始码的一部分）
		if r.getByteAt(i) == 0x00 && r.getByteAt(i+1) == 0x00 && r.getByteAt(i+2) == 0x01 {
			// 确保这不是4字节起始码的一部分
			if i == 0 || r.getByteAt(i-1) != 0x00 {
				return i, 3, true
			}
		}
	}

	return 0, 0, false
}

// getByteAt 获取指定位置的字节，不改变读取位置
func (r *AnnexBReader) getByteAt(pos int) byte {
	if pos >= r.Length {
		return 0
	}

	// 计算在哪个缓冲区和缓冲区内的位置
	currentPos := 0
	bufferIndex := r.offset0
	bufferOffset := r.offset1

	for bufferIndex < len(r.Buffers) {
		buf := r.Buffers[bufferIndex]
		available := len(buf) - bufferOffset

		if currentPos+available > pos {
			// 目标位置在当前缓冲区内
			return buf[bufferOffset+(pos-currentPos)]
		}

		currentPos += available
		bufferIndex++
		bufferOffset = 0
	}

	return 0
}

type InvalidDataError struct {
	util.Memory
}

func (e InvalidDataError) Error() string {
	return fmt.Sprintf("% 02X", e.ToBytes())
}

// ReadNALU 读取一个完整的 NALU
// withStart 用于接收“包含起始码”的内存段
// withoutStart 用于接收“不包含起始码”的内存段
// 允许 withStart 或 withoutStart 为 nil（表示调用方不需要该形式的数据）
func (r *AnnexBReader) ReadNALU(withStart, withoutStart *util.Memory) error {
	r.ClipFront()
	// 定位到第一个起始码
	firstPos, startCodeLen, found := r.FindStartCode()
	if !found {
		return nil
	}

	// 跳过起始码之前的无效数据
	if firstPos > 0 {
		var invalidData util.Memory
		var reader util.MemoryReader
		reader.Memory = &r.Memory
		reader.RangeN(firstPos, invalidData.PushOne)
		return InvalidDataError{invalidData}
	}

	// 为了查找下一个起始码，需要临时跳过当前起始码再查找
	saveOffset0, saveOffset1, saveLength := r.offset0, r.offset1, r.Length
	r.forward(startCodeLen)
	nextPosAfterStart, _, nextFound := r.FindStartCode()
	// 恢复到起始码起点
	r.offset0, r.offset1, r.Length = saveOffset0, saveOffset1, saveLength
	if !nextFound {
		return nil
	}

	// 依次读取并填充输出，同时推进读取位置到 NALU 末尾（不消耗下一个起始码）
	remaining := startCodeLen + nextPosAfterStart
	// 需要在 withoutStart 中跳过的前缀（即起始码长度）
	skipForWithout := startCodeLen

	for remaining > 0 && r.offset0 < len(r.Buffers) {
		buf := r.getCurrentBuffer()
		readLen := len(buf)
		if readLen > remaining {
			readLen = remaining
		}
		segment := buf[:readLen]

		if withStart != nil {
			withStart.PushOne(segment)
		}

		if withoutStart != nil {
			if skipForWithout >= readLen {
				// 本段全部属于起始码，跳过
				skipForWithout -= readLen
			} else {
				// 仅跳过起始码前缀，余下推入 withoutStart
				withoutStart.PushOne(segment[skipForWithout:])
				skipForWithout = 0
			}
		}

		if readLen == len(buf) {
			r.skipCurrentBuffer()
		} else {
			r.forward(readLen)
		}
		remaining -= readLen
	}

	return nil
}

// getCurrentBuffer 获取当前读取位置的缓冲区
func (r *AnnexBReader) getCurrentBuffer() []byte {
	if r.offset0 >= len(r.Buffers) {
		return nil
	}
	return r.Buffers[r.offset0][r.offset1:]
}

// forward 向前移动读取位置
func (r *AnnexBReader) forward(n int) {
	if n <= 0 || r.Length <= 0 {
		return
	}
	if n > r.Length { // 防御：不允许超出剩余长度
		n = r.Length
	}
	r.Length -= n
	for n > 0 && r.offset0 < len(r.Buffers) {
		cur := r.Buffers[r.offset0]
		remain := len(cur) - r.offset1
		if n < remain { // 仍在当前缓冲区内
			r.offset1 += n
			n = 0
			return
		}
		// 用掉当前缓冲区剩余部分，跳到下一个缓冲区起点
		n -= remain
		r.offset0++
		r.offset1 = 0
	}
}

// skipCurrentBuffer 跳过当前缓冲区
func (r *AnnexBReader) skipCurrentBuffer() {
	if r.offset0 < len(r.Buffers) {
		curBufLen := len(r.Buffers[r.offset0]) - r.offset1
		r.Length -= curBufLen
		r.offset0++
		r.offset1 = 0
	}
}
