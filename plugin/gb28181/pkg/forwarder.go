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
	FeedChan    chan []byte  // 接收RTP数据的通道
	RTPReader   *rtp2.TCP    // RTP TCP读取器
	ListenAddr  string       // 监听地址
	ListenPort  uint16       // 监听端口
	listener    net.Listener // TCP监听器
	udpListener *net.UDPConn // UDP监听器
	// 是否为TCP传输
	TCP bool
	// 是否为TCP主动模式
	TCPActive    bool
	TargetIP     string        // 目标IP地址
	TargetPort   int           // 目标端口
	TargetSSRC   string        // 目标SSRC，用于替换RTP包中的SSRC
	udpConn      *net.UDPConn  // UDP发送连接
	tcpConn      net.Conn      // TCP发送连接
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

	// 检查是否已经停止
	select {
	case <-p.stopChan:
		// 已经收到停止信号，不再发送数据
		return nil
	default:
		// 将完整的RTP包数据发送到通道
		select {
		case p.FeedChan <- rtpData:
			// 成功发送
		case <-p.stopChan:
			// 发送过程中收到停止信号
			return nil
		default:
			// 通道已满，记录警告
			p.Warn("feed channel full, dropping packet")
		}
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

	// 检查是否已经停止
	select {
	case <-p.stopChan:
		// 已经收到停止信号，不再发送数据
		return nil
	default:
		// 将完整的RTP包数据发送到通道
		select {
		case p.FeedChan <- rtpData:
			// 成功发送
		case <-p.stopChan:
			// 发送过程中收到停止信号
			return nil
		default:
			// 通道已满，记录警告
			p.Warn("feed channel full, dropping packet")
		}
	}

	return nil
}

// SetTarget 设置转发目标地址
func (p *RTPForwarder) SetTarget(ip string, port int) error {
	p.TargetIP = ip
	p.TargetPort = port

	// 根据转发协议创建相应的连接
	if !p.TCP {
		// 关闭已存在的UDP连接
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
	} else {
		if p.TCPActive {

		} else {
			// 关闭已存在的TCP连接
			if p.tcpConn != nil {
				p.tcpConn.Close()
			}

			// 创建新的TCP连接
			addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
			var err error
			p.tcpConn, err = net.Dial("tcp", addr)
			if err != nil {
				p.Error("dial tcp error", "err", err)
				return err
			}
		}
	}
	p.Info("set target success", "ip", ip, "port", port, "TCP", p.TCP, "TCPActive", p.TCPActive)
	return nil
}

// Start 启动监听
func (p *RTPForwarder) Start() (err error) {
	p.Info("RTPForwarder start", "target", p.TargetIP, "port", p.TargetPort)
	if p.TCP {
		p.listener, err = net.Listen("tcp4", p.ListenAddr)
		if err != nil {
			p.Error("start tcp listen error", "err", err)
			return
		}
		p.Info("start tcp listen", "addr", p.ListenAddr)
	} else {
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
	}
	p.Info("RTPForwarder end")
	return nil
}

// Go 启动处理任务
func (p *RTPForwarder) Go() error {
	p.Info("start go", "addr", p.ListenAddr)
	//if p.TCP {
	return p.goTCP()
	//} else {
	//	return p.goUDP()
	//}
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

				// 创建一个本地的停止通道，用于监听全局停止信号
				localStopChan := make(chan struct{})

				// 监听全局停止信号
				go func() {
					select {
					case <-p.stopChan:
						// 收到全局停止信号，关闭连接
						tcpConn.Close()
						close(localStopChan)
					}
				}()

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

// Demux 阻塞读取RTP并转发至目标IP和端口
func (p *RTPForwarder) Demux() {
	defer p.Info("demux exit")

	// 检查是否设置了目标地址
	if !p.TCP && p.udpConn == nil {
		p.Error("no udp target set for forwarding")
		return
	}

	if p.TCP && p.tcpConn == nil {
		p.Error("no tcp target set for forwarding")
		return
	}

	p.Info("start demux and forward",
		"target", net.JoinHostPort(p.TargetIP, fmt.Sprintf("%d", p.TargetPort)),
		"TCP", p.TCP, "TCPActive", p.TCPActive)

	// 持续从FeedChan读取RTP数据并转发
	for rtpData := range p.FeedChan {
		var err error

		// 根据转发协议选择不同的发送方式
		if !p.TCP {
			// 确保发送的是标准RTP包
			// 检查是否是有效的RTP包
			packet := &rtp.Packet{}
			if parseErr := packet.Unmarshal(rtpData); parseErr != nil {
				p.Error("invalid RTP packet for UDP forwarding", "err", parseErr)
				continue
			}

			// 如果设置了目标SSRC，则修改RTP包中的SSRC
			if p.TargetSSRC != "" {
				targetSSRCUint, err := strconv.ParseUint(p.TargetSSRC, 10, 32)
				if err == nil {
					// 修改SSRC
					packet.SSRC = uint32(targetSSRCUint)

					// 重新编码RTP包
					modifiedData, err := packet.Marshal()
					if err == nil {
						// 发送修改后的RTP包
						_, err = p.udpConn.Write(modifiedData)
					} else {
						p.Error("marshal modified rtp packet error", "err", err)
						// 发送原始RTP包
						_, err = p.udpConn.Write(rtpData)
					}
				} else {
					p.Error("parse target ssrc error", "err", err)
					// 发送原始RTP包
					_, err = p.udpConn.Write(rtpData)
				}
			} else {
				// 直接发送原始RTP包
				_, err = p.udpConn.Write(rtpData)
			}
		} else {
			// 对于TCP，需要添加2字节的长度前缀
			if p.tcpConn != nil {
				// 创建带长度前缀的数据包
				tcpData := make([]byte, len(rtpData)+2)
				// 设置长度前缀（大端序）
				tcpData[0] = byte((len(rtpData) >> 8) & 0xFF)
				tcpData[1] = byte(len(rtpData) & 0xFF)
				// 复制RTP数据
				copy(tcpData[2:], rtpData)

				// 发送到TCP连接
				_, err = p.tcpConn.Write(tcpData)
			} else {
				err = fmt.Errorf("tcp connection not established")
			}
		}

		if err != nil {
			p.Error("forward rtp packet error", "err", err, "TCP", p.TCP, "TCPActive", p.TCPActive)
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
			p.Trace("forward rtp packet", "count", p.ForwardCount, "TCP", p.TCP, "TCPActive", p.TCPActive)
		}
	}
}

// Dispose 释放资源
func (p *RTPForwarder) Dispose() {
	p.Info("disposing forwarder")

	// 发送停止信号
	close(p.stopChan)

	// 给一些时间让所有goroutine响应停止信号
	time.Sleep(100 * time.Millisecond)

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

	if p.tcpConn != nil {
		p.tcpConn.Close()
	}

	// 确保所有goroutine都有机会处理停止信号后再关闭FeedChan
	close(p.FeedChan)
	p.Info("forwarder disposed", "forwarded_packets", p.ForwardCount)
}
