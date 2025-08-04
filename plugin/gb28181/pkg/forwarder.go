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
2. 使用前需设置监听地址(DownListenAddr)和转发目标(SetTarget)
3. 资源使用完毕后需调用Dispose方法释放资源
*/

package gb28181

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pion/rtp"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

// RTPForwarder 接收RTP数据包并转发到指定目标的结构体
type RTPForwarder struct {
	task.Job
	rtp.Packet
	FeedChan       chan []byte  // 接收RTP数据的通道
	UpListenAddr   string       //用于发送上级设备的监听地址
	upListener     net.Listener //用于发送上级设备的TCP监听器
	DownListenAddr string       // 用于接收下级摄像头数据监听地址
	downListener   net.Listener // 用于接收下级摄像头数据的TCP监听器
	udpListener    *net.UDPConn // UDP监听器
	// 是否为TCP传输
	TCP bool
	// 是否为TCP主动模式
	TCPActive    bool
	TargetIP     string        // 目标IP地址
	TargetPort   int           // 目标端口
	TargetSSRC   string        // 目标SSRC，用于替换RTP包中的SSRC
	udpConn      *net.UDPConn  // UDP发送连接
	tcpConn      net.Conn      // TCP发送连接
	ForwardCount int64         // 已转发的包数量
	SendInterval time.Duration // 发送间隔，可用于限流
	lastSendTime time.Time     // 上次发送时间
	stopChan     chan struct{} // 停止信号通道
	StreamMode   string        // 数据流传输模式（UDP:udp传输/TCP-ACTIVE：tcp主动模式/TCP-PASSIVE：tcp被动模式）
}

// NewRTPForwarder 创建一个新的RTP转发器
func NewRTPForwarder() *RTPForwarder {
	ret := &RTPForwarder{
		FeedChan:     make(chan []byte, 2000), // 增加缓冲区大小，减少丢包风险
		SendInterval: time.Millisecond * 0,    // 默认不限制发送间隔，最大速度转发
		stopChan:     make(chan struct{}),
	}
	return ret
}

// ReadRTP 读取RTP包
func (p *RTPForwarder) ReadRTP(rtpBuf util.Buffer) (err error) {
	if err = p.Unmarshal(rtpBuf); err != nil {
		p.Error("unmarshal error", "err", err)
		return
	}

	if p.TraceEnabled() {
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

		p.Info("start udp to up platform", "ip", ip, "port", port)
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
		go func() {
			// 如果是TCP主动模式且还没有建立连接，等待连接
			p.Info("start to accept uplistener", "p.UpListenAddr", p.UpListenAddr, "tcpConn is", p.tcpConn == nil, "p.Tcp is", p.TCP, "p.TCPActive", p.TCPActive)
			if p.TCP && p.TCPActive && p.tcpConn == nil {
				var err error
				if p.upListener == nil {
					p.upListener, err = net.Listen("tcp4", p.UpListenAddr)
					if err != nil {
						p.Error("start udp listen error", "err", err)
					}
				}
				p.Info("waiting for upstream connection...")
				p.tcpConn, err = p.upListener.Accept()
				if err != nil {
					p.Error("accept upstream connection failed", "err", err)
				}
				p.Info("upstream connected", "addr", p.tcpConn.RemoteAddr())
			}
		}()
	}
	p.Info("set target success", "ip", ip, "port", port, "TCP", p.TCP, "TCPActive", p.TCPActive)
	return nil
}

// Start 启动监听
func (p *RTPForwarder) Start() (err error) {
	p.Info("RTPForwarder start", "target", p.TargetIP, "port", p.TargetPort)
	if strings.ToUpper(p.StreamMode) == "TCP-ACTIVE" {
		// TCP主动模式不需要监听，直接返回
		p.Info("TCP-ACTIVE mode, no need to listen")
	} else if strings.ToUpper(p.StreamMode) == "TCP-PASSIVE" {
		p.downListener, err = net.Listen("tcp4", p.DownListenAddr)
		if err != nil {
			p.Error("start tcp listen error", "err", err)
			return err
		}
		p.Info("start tcp down listen", "streammode", p.StreamMode, "addr", p.DownListenAddr)
	} else {
		addr, err := net.ResolveUDPAddr("udp", p.DownListenAddr)
		if err != nil {
			p.Error("resolve udp addr error", "err", err)
			return err
		}
		p.udpListener, err = net.ListenUDP("udp", addr)
		if err != nil {
			p.Error("start udp listen error", "err", err)
			return err
		}
		p.Info("start udp listen", "addr", p.DownListenAddr)
	}

	if !p.TCPActive && p.TCP { //TCP被动模式，需要服务器主动连接上级设备
		// 创建新的TCP连接
		addr := p.UpListenAddr
		var err error
		p.tcpConn, err = net.Dial("tcp", addr)
		p.Info("start tcp listen,now is tcp-passive", "addr", addr)
		if err != nil {
			p.Error("dial tcp error", "err", err)
			return err
		}
	}
	p.goTCP()
	p.Info("RTPForwarder end")
	return nil
}

// goTCP 处理TCP连接的RTP包
func (p *RTPForwarder) goTCP() error {
	p.Info("start tcp accept")
	if strings.ToUpper(p.StreamMode) == "TCP-ACTIVE" {
		// var active mrtp.ReceiveTCPActive
		// active.Receiver = p
		// active.ListenAddr = p.DownListenAddr
		// p.AddTask(&active)
		return nil
	}
	if p.downListener == nil {
		p.Error("downListener is nil, cannot accept TCP connections")
		return fmt.Errorf("downListener is nil, cannot accept TCP connections")
	}
	// var passive mrtp.ReceiveTCPPassive
	// passive.Listener = p.downListener
	// passive.Receiver = p
	// p.AddTask(&passive)
	p.Info("start tcp down listen", "streammode", p.StreamMode, "addr", p.DownListenAddr)
	return nil
}

// Demux 阻塞读取RTP并转发至目标IP和端口
func (p *RTPForwarder) Demux() {
	defer p.Info("demux exit")

	// 检查是否设置了目标地址
	if !p.TCP && p.udpConn == nil {
		p.Error("no udp target set for forwarding")
		return
	}

	//if p.TCP && p.tcpConn == nil {
	//	p.Error("no tcp target set for forwarding")
	//	return
	//}

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

		if p.TraceEnabled() && p.ForwardCount%1000 == 0 {
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

	if p.downListener != nil {
		p.downListener.Close()
	}

	if p.upListener != nil {
		p.upListener.Close()
	}

	if p.udpListener != nil {
		p.udpListener.Close()
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
