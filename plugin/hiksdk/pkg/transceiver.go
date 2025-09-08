package pkg

import (
	"fmt"
	"io"

	mpegps "m7s.live/v5/pkg/format/ps"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

type ChanReader chan []byte

func (r ChanReader) Read(buf []byte) (n int, err error) {
	b, ok := <-r
	if !ok {
		return 0, io.EOF
	}
	copy(buf, b)
	return len(b), nil
}

type Receiver struct {
	task.Task
	*util.BufReader
	SSRC     uint32      // RTP SSRC
	PSMouth  chan []byte // 直接处理PS数据的通道
	psBuffer []byte      // PS数据缓冲区，用于处理跨包的PS起始码
}

type PSReceiver struct {
	Device    Device // 设备
	ChannelId int    // 通道号
	Receiver
	mpegps.MpegPsDemuxer
}

func (p *PSReceiver) Start() error {
	err := p.Receiver.Start()
	if err == nil {
		p.Using(p.Publisher)
	}
	// 初始化播放控制通道（如果未初始化）
	p.Device.RealPlay_V40(p.ChannelId, &p.Receiver)
	return err
}

func (p *PSReceiver) Run() error {
	p.MpegPsDemuxer.Allocator = util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	p.Using(p.MpegPsDemuxer.Allocator)

	// 确保Publisher已设置
	if p.MpegPsDemuxer.Publisher == nil {
		return fmt.Errorf("Publisher未设置")
	}

	err := p.MpegPsDemuxer.Feed(p.BufReader)
	return err
}

func (p *PSReceiver) Dispose() {
	p.Device.StopRealPlay()
	// 停止设备播放
}

func (p *Receiver) Start() (err error) {
	p.PSMouth = make(chan []byte, 500)  // 增加PS数据通道缓冲区到500，避免数据丢失
	psReader := (ChanReader)(p.PSMouth) // 直接使用PS数据通道
	p.BufReader = util.NewBufReader(psReader)
	return
}

func (p *Receiver) ReadPSData(data util.Buffer) (err error) {
	// 将新数据添加到缓冲区
	p.psBuffer = append(p.psBuffer, data...)

	// 处理缓冲区中的完整PS包
	for {
		syncedData, remaining := p.extractSynchronizedPSData(p.psBuffer)
		if syncedData == nil {
			// 没有找到完整的PS包，保留剩余数据等待更多数据
			p.psBuffer = remaining
			break
		}

		// 发送同步后的PS数据到处理通道
		select {
		case p.PSMouth <- syncedData:
			// 成功发送数据到PS处理通道
			// fmt.Printf("发送同步PS数据到通道，长度: %d\n", len(syncedData))
		default:
			// 通道满了，跳过这个数据包
			fmt.Printf("PS通道满了，跳过数据包，当前缓冲区大小: %d/%d\n", len(p.PSMouth), cap(p.PSMouth))
			// 跳过当前数据包，但不返回错误，避免阻塞
		}

		// 更新缓冲区为剩余数据
		p.psBuffer = remaining
	}

	return nil
}

// extractSynchronizedPSData 从缓冲区中提取同步的PS数据包
func (p *Receiver) extractSynchronizedPSData(buffer []byte) ([]byte, []byte) {
	if len(buffer) < 4 {
		return nil, buffer // 数据不足，返回所有数据等待更多
	}

	// 寻找PS起始码
	startIndex := -1
	for i := 0; i <= len(buffer)-4; i++ {
		if buffer[i] == 0x00 && buffer[i+1] == 0x00 && buffer[i+2] == 0x01 {
			// 检查第四个字节是否为有效的PS起始码
			startCode := uint32(buffer[i])<<24 | uint32(buffer[i+1])<<16 |
				uint32(buffer[i+2])<<8 | uint32(buffer[i+3])

			switch startCode {
			case mpegps.StartCodePS, mpegps.StartCodeVideo, mpegps.StartCodeVideo1,
				mpegps.StartCodeVideo2, mpegps.StartCodeAudio, mpegps.StartCodeMAP,
				mpegps.StartCodeSYS, mpegps.PrivateStreamCode:
				startIndex = i
				// fmt.Println("在数据源头找到PS起始码:", fmt.Sprintf("0x%08x", startCode))
				goto found
			}
		}
	}

found:
	if startIndex == -1 {
		// 没有找到有效起始码
		if len(buffer) > 3 {
			// 保留最后3个字节，丢弃其余数据
			return nil, buffer[len(buffer)-3:]
		}
		return nil, buffer
	}

	// 寻找下一个起始码来确定当前包的结束位置
	nextStartIndex := -1
	for i := startIndex + 4; i <= len(buffer)-4; i++ {
		if buffer[i] == 0x00 && buffer[i+1] == 0x00 && buffer[i+2] == 0x01 {
			startCode := uint32(buffer[i])<<24 | uint32(buffer[i+1])<<16 |
				uint32(buffer[i+2])<<8 | uint32(buffer[i+3])

			switch startCode {
			case mpegps.StartCodePS, mpegps.StartCodeVideo, mpegps.StartCodeVideo1,
				mpegps.StartCodeVideo2, mpegps.StartCodeAudio, mpegps.StartCodeMAP,
				mpegps.StartCodeSYS, mpegps.PrivateStreamCode:
				nextStartIndex = i
				goto nextFound
			}
		}
	}

nextFound:
	if nextStartIndex == -1 {
		// 没有找到下一个起始码，返回从当前起始码到缓冲区末尾的所有数据
		return buffer[startIndex:], nil
	}

	// 返回从当前起始码到下一个起始码之间的数据
	return buffer[startIndex:nextStartIndex], buffer[nextStartIndex:]
}
