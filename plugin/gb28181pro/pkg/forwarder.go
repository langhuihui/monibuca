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
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pion/rtp"
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
	TargetSSRC   string        // 目标SSRC，用于替换RTP包中的SSRC
	udpConn      *net.UDPConn  // UDP发送连接
	bufferPool   sync.Pool     // 缓冲池
	ForwardCount int64         // 已转发的包数量
	SendInterval time.Duration // 发送间隔，可用于限流
	lastSendTime time.Time     // 上次发送时间
	stopChan     chan struct{} // 停止信号通道
	*slog.Logger
}

// NewRTPForwarder 创建一个新的RTP转发器
func NewRTPForwarder() *RTPForwarder {
	ret := &RTPForwarder{
		FeedChan:     make(chan []byte, 2000), // 增加缓冲区大小，减少丢包风险
		SendInterval: time.Millisecond * 0,    // 默认不限制发送间隔，最大速度转发
		Protocol:     "tcp",                   // 默认使用TCP
		stopChan:     make(chan struct{}),
		Logger:       slog.New(slog.NewTextHandler(os.Stdout, nil)),
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
	if err = p.Unmarshal(rtpBuf); err != nil {
		p.Error("unmarshal error", "err", err)
		return
	}

	if p.Enabled(p, task.TraceLevel) {
		p.Trace("rtp", "len", rtpBuf.Len(), "seq", p.SequenceNumber, "payloadType", p.PayloadType, "ssrc", p.SSRC)
	}

	// 直接使用原始RTP包数据
	rtpData := make([]byte, rtpBuf.Len())
	copy(rtpData, rtpBuf)

	// 将完整的RTP包数据发送到通道
	select {
	case p.FeedChan <- rtpData:
		// 成功发送
	default:
		// 通道已满，记录警告
		p.Warn("feed channel full, dropping packet")
	}

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

	if p.Enabled(p, task.TraceLevel) {
		p.Trace("rtp bytes", "len", len(data), "seq", packet.SequenceNumber, "payloadType", packet.PayloadType, "ssrc", packet.SSRC)
	}

	// 直接使用原始RTP包数据
	rtpData := make([]byte, len(data))
	copy(rtpData, data)

	// 将完整的RTP包数据发送到通道
	select {
	case p.FeedChan <- rtpData:
		// 成功发送
	default:
		// 通道已满，记录警告
		p.Warn("feed channel full, dropping packet")
	}

	return nil
}

// ForwardRTPPacket 转发RTP包到目标地址
func (p *RTPForwarder) ForwardRTPPacket(rtpBuf util.Buffer) {
	// 限流控制
	if !p.lastSendTime.IsZero() && time.Since(p.lastSendTime) < p.SendInterval {
		time.Sleep(p.SendInterval - time.Since(p.lastSendTime))
	}

	// 如果设置了目标SSRC，则修改RTP包中的SSRC
	if p.TargetSSRC != "" {
		// 创建一个新的RTP包
		packet := &rtp.Packet{}

		// 解析原始RTP包
		if err := packet.Unmarshal(rtpBuf.Bytes()); err == nil {
			// 将字符串SSRC转换为uint32
			targetSSRCUint, err := strconv.ParseUint(p.TargetSSRC, 10, 32)
			if err != nil {
				p.Error("parse target ssrc error", "err", err)
				// 发送原始RTP包
				_, err = p.udpConn.Write(rtpBuf.Bytes())
				if err != nil {
					p.Error("forward original rtp packet error", "err", err)
				}
				return
			}

			// 修改SSRC
			packet.SSRC = uint32(targetSSRCUint)

			// 重新编码RTP包
			modifiedData, err := packet.Marshal()
			if err == nil {
				// 发送修改后的RTP包
				_, err = p.udpConn.Write(modifiedData)
				if err != nil {
					p.Error("forward modified rtp packet error", "err", err)
					return
				}
			} else {
				p.Error("marshal modified rtp packet error", "err", err)
				// 发送原始RTP包
				_, err = p.udpConn.Write(rtpBuf.Bytes())
				if err != nil {
					p.Error("forward original rtp packet error", "err", err)
					return
				}
			}
		} else {
			p.Error("unmarshal rtp packet error", "err", err)
			// 发送原始RTP包
			_, err = p.udpConn.Write(rtpBuf.Bytes())
			if err != nil {
				p.Error("forward original rtp packet error", "err", err)
				return
			}
		}
	} else {
		// 直接发送原始RTP包
		_, err := p.udpConn.Write(rtpBuf.Bytes())
		if err != nil {
			p.Error("forward rtp packet error", "err", err)
			return
		}
	}

	p.lastSendTime = time.Now()
	p.ForwardCount++

	if p.Enabled(p, task.TraceLevel) && p.ForwardCount%1000 == 0 {
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
	p.Info("RTPForwarder start", "target", p.TargetIP, "port", p.TargetPort)
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
	p.Info("RTPForwarder end")
	return nil
}

// Go 启动处理任务
func (p *RTPForwarder) Go() error {
	p.Info("start go", "addr", p.ListenAddr)
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

	// 创建一个停止信号通道，与goUDP保持一致
	if p.stopChan == nil {
		p.stopChan = make(chan struct{})
	}

	// 持续接受新的TCP连接
	for {
		select {
		case <-p.stopChan:
			return nil
		default:
			// 设置接受连接的超时时间，避免阻塞
			p.listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))

			conn, err := p.listener.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // 超时，继续接收
				}
				p.Error("accept error", "err", err)
				return err
			}

			// 为每个连接创建一个goroutine来处理
			go func(conn net.Conn) {
				tcpConn := conn.(*net.TCPConn)
				p.Info("accept", "addr", conn.RemoteAddr())

				// 创建RTP TCP读取器
				rtpReader := (*rtp2.TCP)(tcpConn)

				// 开始读取RTP包
				err := rtpReader.Read(p.ReadRTP)
				if err != nil {
					p.Error("read rtp error", "err", err)
				}

				// 关闭连接
				tcpConn.Close()
			}(conn)
		}
	}
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

// Demux 阻塞读取RTP并转发至目标IP和端口
func (p *RTPForwarder) Demux() {
	defer p.Info("demux exit")

	// 检查是否设置了目标地址
	if p.udpConn == nil {
		p.Error("no target set for forwarding")
		return
	}

	p.Info("start demux and forward", "target", net.JoinHostPort(p.TargetIP, fmt.Sprintf("%d", p.TargetPort)))

	// 持续从FeedChan读取RTP数据并转发
	for rtpData := range p.FeedChan {
		// 直接转发原始RTP包数据
		_, err := p.udpConn.Write(rtpData)
		if err != nil {
			p.Error("forward rtp packet error", "err", err)
			continue
		}

		p.ForwardCount++

		// 控制发送速率
		if p.SendInterval > 0 && !p.lastSendTime.IsZero() {
			elapsed := time.Since(p.lastSendTime)
			if elapsed < p.SendInterval {
				time.Sleep(p.SendInterval - elapsed)
			}
		}
		p.lastSendTime = time.Now()

		if p.Enabled(p, task.TraceLevel) && p.ForwardCount%1000 == 0 {
			p.Trace("forward rtp packet", "count", p.ForwardCount)
		}
	}
}
