/*
RTPForwarder 是一个RTP包转发器，主要功能包括：

1. 可通过TCP或UDP协议接收RTP包
2. 接收RTP包后不进行解析，直接转发到指定的IP和端口
3. 支持限流控制，可设置发送间隔
4. 提供与Monibuca系统集成的Publisher接口
5. 提供了UDP和TCP两种模式的使用示例

使用场景：
1. 作为GB28181协议中的媒体接收和转发节点
2. 在不需要解析媒体内容的情况下，实现RTP流的中转
3. 可用于搭建分发网络，将接收到的RTP流转发到多个目标

注意事项：
1. 默认使用TCP协议，可通过设置Protocol字段切换为UDP模式
2. 使用前需设置监听地址(ListenAddr)和转发目标(SetTarget)
3. 资源使用完毕后需调用Dispose方法释放资源
*/

package gb28181

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	rtp2 "m7s.live/v5/plugin/rtp/pkg"
)

// RTPForwarder 接收RTP数据包并转发到指定目标的结构体
type RTPForwarder struct {
	task.Task
	rtp.Packet
	FeedChan     chan []byte   // 接收RTP数据的通道
	RTPReader    *rtp2.TCP     // RTP TCP读取器
	ListenAddr   string        // 监听地址
	ListenPort   uint16        // 监听端口
	listener     net.Listener  // TCP监听器
	udpListener  *net.UDPConn  // UDP监听器
	Protocol     string        // 监听协议: "tcp" 或 "udp"
	TargetIP     string        // 目标IP地址
	TargetPort   int           // 目标端口
	udpConn      *net.UDPConn  // UDP发送连接
	bufferPool   sync.Pool     // 缓冲池
	ForwardCount int64         // 已转发的包数量
	SendInterval time.Duration // 发送间隔，可用于限流
	lastSendTime time.Time     // 上次发送时间
	stopChan     chan struct{} // 停止信号通道
}

// NewRTPForwarder 创建一个新的RTP转发器
func NewRTPForwarder() *RTPForwarder {
	ret := &RTPForwarder{
		FeedChan:     make(chan []byte, 100),
		SendInterval: time.Millisecond * 5, // 默认5ms间隔，可根据需要调整
		Protocol:     "tcp",                // 默认使用TCP
		stopChan:     make(chan struct{}),
	}

	ret.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 1500) // 常见MTU大小
		},
	}

	return ret
}

// ReadRTP 读取RTP包
func (p *RTPForwarder) ReadRTP(rtpBuf util.Buffer) (err error) {
	lastSeq := p.SequenceNumber

	if err = p.Unmarshal(rtpBuf); err != nil {
		p.Error("unmarshal error", "err", err)
		return
	}

	// 检查序列号连续性
	if lastSeq == 0 || p.SequenceNumber == lastSeq+1 {
		if p.Enabled(p, task.TraceLevel) {
			p.Trace("rtp", "len", rtpBuf.Len(), "seq", p.SequenceNumber, "payloadType", p.PayloadType, "ssrc", p.SSRC)
		}

		// 创建一个副本，以防在异步处理时缓冲区被重用
		copyData := make([]byte, len(p.Payload))
		copy(copyData, p.Payload)

		// 将数据发送到通道
		p.FeedChan <- copyData

		// 直接转发RTP包到目标地址（如果已设置）
		if p.udpConn != nil {
			p.ForwardRTPPacket(rtpBuf)
		}

		return nil
	}

	p.Warn("rtp sequence discontinuity", "expected", lastSeq+1, "got", p.SequenceNumber)
	return nil
}

// ReadRTPBytes 读取RTP包的字节数组版本
func (p *RTPForwarder) ReadRTPBytes(data []byte) (err error) {
	packet := rtp.Packet{}
	if err = packet.Unmarshal(data); err != nil {
		p.Error("unmarshal bytes error", "err", err)
		return
	}

	// 设置当前Packet的值
	p.Packet = packet

	// 创建一个副本，以防在异步处理时缓冲区被重用
	copyData := make([]byte, len(p.Payload))
	copy(copyData, p.Payload)

	// 将数据发送到通道
	p.FeedChan <- copyData

	// 直接转发RTP包到目标地址（如果已设置）
	if p.udpConn != nil {
		_, err = p.udpConn.Write(data)
		if err != nil {
			p.Error("forward rtp bytes error", "err", err)
			return
		}

		p.lastSendTime = time.Now()
		p.ForwardCount++

		if p.Enabled(p, task.TraceLevel) && p.ForwardCount%100 == 0 {
			p.Trace("forward rtp packet", "count", p.ForwardCount)
		}
	}

	return nil
}

// ForwardRTPPacket 将RTP包转发到目标地址
func (p *RTPForwarder) ForwardRTPPacket(rtpBuf util.Buffer) {
	// 限流控制
	if !p.lastSendTime.IsZero() && time.Since(p.lastSendTime) < p.SendInterval {
		time.Sleep(p.SendInterval - time.Since(p.lastSendTime))
	}

	_, err := p.udpConn.Write(rtpBuf.Bytes())
	if err != nil {
		p.Error("forward rtp packet error", "err", err)
		return
	}

	p.lastSendTime = time.Now()
	p.ForwardCount++

	if p.Enabled(p, task.TraceLevel) && p.ForwardCount%100 == 0 {
		p.Trace("forward rtp packet", "count", p.ForwardCount)
	}
}

// SetTarget 设置转发目标地址
func (p *RTPForwarder) SetTarget(ip string, port int) error {
	p.TargetIP = ip
	p.TargetPort = port

	// 关闭已存在的连接
	if p.udpConn != nil {
		p.udpConn.Close()
	}

	// 创建新的UDP连接
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
	if err != nil {
		p.Error("resolve udp addr error", "err", err)
		return err
	}

	p.udpConn, err = net.DialUDP("udp", nil, addr)
	if err != nil {
		p.Error("dial udp error", "err", err)
		return err
	}

	p.Info("set target success", "ip", ip, "port", port)
	return nil
}

// Start 启动监听
func (p *RTPForwarder) Start() (err error) {
	switch p.Protocol {
	case "tcp", "TCP":
		p.listener, err = net.Listen("tcp4", p.ListenAddr)
		if err != nil {
			p.Error("start tcp listen error", "err", err)
			return
		}
		p.Info("start tcp listen", "addr", p.ListenAddr)
	case "udp", "UDP":
		addr, err := net.ResolveUDPAddr("udp", p.ListenAddr)
		if err != nil {
			p.Error("resolve udp addr error", "err", err)
			return err
		}
		p.udpListener, err = net.ListenUDP("udp", addr)
		if err != nil {
			p.Error("start udp listen error", "err", err)
			return err
		}
		p.Info("start udp listen", "addr", p.ListenAddr)
	default:
		return fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}

	return nil
}

// Go 启动处理任务
func (p *RTPForwarder) Go() error {
	switch p.Protocol {
	case "tcp", "TCP":
		return p.goTCP()
	case "udp", "UDP":
		return p.goUDP()
	default:
		return fmt.Errorf("unsupported protocol: %s", p.Protocol)
	}
}

// goTCP 处理TCP连接的RTP包
func (p *RTPForwarder) goTCP() error {
	p.Info("start tcp accept")
	conn, err := p.listener.Accept()
	if err != nil {
		p.Error("accept error", "err", err)
		return err
	}

	p.RTPReader = (*rtp2.TCP)(conn.(*net.TCPConn))
	p.Info("accept", "addr", conn.RemoteAddr())

	// 开始读取RTP包
	return p.RTPReader.Read(p.ReadRTP)
}

// goUDP 处理UDP连接的RTP包
func (p *RTPForwarder) goUDP() error {
	p.Info("start udp receive")

	buffer := make([]byte, 1500)

	for {
		select {
		case <-p.stopChan:
			return nil
		default:
			p.udpListener.SetReadDeadline(time.Now().Add(time.Second))
			n, _, err := p.udpListener.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // 超时，继续接收
				}
				p.Error("udp read error", "err", err)
				return err
			}

			if n <= 0 {
				continue
			}

			// 创建一个副本用于RTP包解析
			rtpBuf := make([]byte, n)
			copy(rtpBuf, buffer[:n])

			// 使用ReadRTPBytes方法处理RTP包
			err = p.ReadRTPBytes(rtpBuf)
			if err != nil {
				p.Error("process rtp packet error", "err", err)
				// 继续接收，不中断
			}
		}
	}
}

// Dispose 释放资源
func (p *RTPForwarder) Dispose() {
	// 发送停止信号
	close(p.stopChan)

	if p.listener != nil {
		p.listener.Close()
	}

	if p.udpListener != nil {
		p.udpListener.Close()
	}

	if p.RTPReader != nil {
		p.RTPReader.Close()
	}

	if p.udpConn != nil {
		p.udpConn.Close()
	}

	close(p.FeedChan)
	p.Info("forwarder disposed", "forwarded_packets", p.ForwardCount)
}

// RTPForwarderPublisher 作为一个发布者
type RTPForwarderPublisher struct {
	*m7s.Publisher
	Forwarder RTPForwarder
}

// NewRTPForwarderPublisher 创建一个新的RTP转发发布者
func NewRTPForwarderPublisher(puber *m7s.Publisher) *RTPForwarderPublisher {
	ret := &RTPForwarderPublisher{
		Publisher: puber,
	}
	ret.Forwarder = *NewRTPForwarder()
	return ret
}

// ExampleUseInDialog 展示如何在Dialog中使用RTPForwarder的示例方法
// 注意：这是示例代码，需要根据实际项目结构进行修改
func ExampleUseInDialog() {
	// 1. 创建一个TCP模式的RTPForwarder
	forwarder := NewRTPForwarder()
	forwarder.Protocol = "tcp" // 默认为TCP模式，可以省略

	// 2. 设置监听地址和端口
	forwarder.ListenAddr = "0.0.0.0:5540" // 根据实际情况设置监听地址

	// 3. 设置目标转发地址和端口
	err := forwarder.SetTarget("192.168.1.100", 8000) // 设置转发目标
	if err != nil {
		// 处理错误
		return
	}

	// 4. 启动监听
	err = forwarder.Start()
	if err != nil {
		// 处理错误
		return
	}

	// 5. 在后台运行转发任务
	go func() {
		err := forwarder.Go()
		if err != nil {
			// 处理错误
		}
	}()

	// 6. 使用完成后，在适当的时候释放资源
	// defer forwarder.Dispose()

	// 7. 如果需要更改转发目标，可以再次调用SetTarget方法
	// forwarder.SetTarget("192.168.1.101", 9000)
}

// ExampleUseUDP 展示如何使用UDP模式的RTPForwarder
func ExampleUseUDP() {
	// 1. 创建一个UDP模式的RTPForwarder
	forwarder := NewRTPForwarder()
	forwarder.Protocol = "udp" // 设置为UDP模式

	// 2. 设置监听地址和端口
	forwarder.ListenAddr = "0.0.0.0:5540"

	// 3. 设置目标转发地址和端口
	err := forwarder.SetTarget("192.168.1.100", 8000)
	if err != nil {
		// 处理错误
		return
	}

	// 4. 启动监听
	err = forwarder.Start()
	if err != nil {
		// 处理错误
		return
	}

	// 5. 在后台运行转发任务
	go func() {
		err := forwarder.Go()
		if err != nil {
			// 处理错误
		}
	}()

	// 6. 使用完成后的清理
	// defer forwarder.Dispose()
}

// ExampleUseWithPublisher 展示如何在Publisher中使用RTPForwarder的示例方法
func ExampleUseWithPublisher(publisherName string) {
	// 1. 假设我们已经获取了一个Publisher实例
	// 在实际应用中，Publisher通常由框架创建或从其他地方获取
	// 这里仅作示例说明，实际使用时需要修改为正确的Publisher获取方式
	var publisher *m7s.Publisher

	// 2. 创建RTPForwarderPublisher
	rtpPublisher := NewRTPForwarderPublisher(publisher)

	// 3. 设置监听地址和端口
	rtpPublisher.Forwarder.ListenAddr = "0.0.0.0:5540"

	// 4. 设置转发目标
	rtpPublisher.Forwarder.SetTarget("192.168.1.100", 8000)

	// 5. 启动监听和处理
	rtpPublisher.Forwarder.Start()
	go rtpPublisher.Forwarder.Go()

	// 6. 使用完成后，释放资源
	// defer rtpPublisher.Forwarder.Dispose()
}
