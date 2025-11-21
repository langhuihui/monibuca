package rtp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/langhuihui/gomem"
	task "github.com/langhuihui/gotask"
	"github.com/pion/rtp"
	mpegps "m7s.live/v5/pkg/format/ps"
	"m7s.live/v5/pkg/util"
)

var ErrRTPReceiveLost = errors.New("rtp receive lost")

// 数据流传输模式（UDP:udp传输、TCP-ACTIVE：tcp主动模式、TCP-PASSIVE：tcp被动模式、MANUAL：手动模式）
type StreamMode string

const (
	StreamModeUDP        StreamMode = "UDP"
	StreamModeTCPActive  StreamMode = "TCP-ACTIVE"
	StreamModeTCPPassive StreamMode = "TCP-PASSIVE"
	StreamModeManual     StreamMode = "MANUAL"
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

type RTPChanReader chan []byte

func (r RTPChanReader) Read(packet *rtp.Packet) (err error) {
	b, ok := <-r
	if !ok {
		return io.EOF
	}
	return packet.Unmarshal(b)
}

func (r RTPChanReader) Close() error {
	close(r)
	return nil
}

type Receiver struct {
	task.Task
	*util.BufReader
	ListenAddr string
	net.Listener
	StreamMode   StreamMode
	RTPMouth     chan []byte
	SinglePort   io.ReadCloser
	rtpReader    *RTPPayloadReader // 保存 RTP 读取器引用
	ExpectedSSRC uint32            // 预期的SSRC（为0则不过滤）
	started      bool              // 标记是否已经启动过
}

type PSReceiver struct {
	Receiver
	mpegps.MpegPsDemuxer
	firstVideoPts        uint64        // 第一个视频帧的 PTS（90kHz）
	currentVideoPts      uint64        // 当前视频帧的 PTS（90kHz）
	hasFirstPts          bool          // 是否已记录第一个 PTS
	lastPtsUpdate        time.Time     // 最后一次 PTS 更新的时间
	OnProgressUpdate     func()        // 进度更新回调（可选，导出供外部使用）
	lastProgressUpdate   time.Time     // 最后一次进度更新时间
	ProgressUpdatePeriod time.Duration // 进度更新周期（默认1秒，导出供外部配置）
}

func (p *PSReceiver) Start() error {
	p.Info("PSReceiver.Start called", "StreamMode", p.Receiver.StreamMode, "SinglePort", p.Receiver.SinglePort != nil, "started", p.Receiver.started, "ListenAddr", p.Receiver.ListenAddr)

	// 多端口模式下始终打印启动日志
	if p.Receiver.StreamMode == StreamModeTCPPassive && p.Receiver.SinglePort == nil {
		if !p.Receiver.started {
			p.Info("start new listener", "addr", p.Receiver.ListenAddr)
		} else {
			p.Info("listener already started", "addr", p.Receiver.ListenAddr)
		}
	}

	err := p.Receiver.Start()
	if err == nil {
		p.Using(p.Publisher)
		// 设置 PTS 更新回调到 MpegPsDemuxer
		p.MpegPsDemuxer.OnVideoPtsUpdate = p.UpdateVideoPts
		// 默认进度更新周期为1秒
		if p.ProgressUpdatePeriod == 0 {
			p.ProgressUpdatePeriod = time.Second
		}
	}
	return err
}

func (p *PSReceiver) Run() error {
	err := p.Receiver.Run()
	if err != nil {
		return err
	}

	p.MpegPsDemuxer.Allocator = gomem.NewScalableMemoryAllocator(1 << gomem.MinPowerOf2)
	p.Using(p.MpegPsDemuxer.Allocator)
	return p.MpegPsDemuxer.Feed(p.BufReader)
}

// UpdateVideoPts 更新视频 PTS（从 MpegPsDemuxer 中调用）
func (p *PSReceiver) UpdateVideoPts(pts uint64) {
	now := time.Now()
	if !p.hasFirstPts {
		p.firstVideoPts = pts
		p.hasFirstPts = true
		p.lastPtsUpdate = now
		p.lastProgressUpdate = now
		p.Info("PSReceiver: 首帧视频PTS", "pts", pts)
	}

	// 检测 PTS 是否变化
	if pts != p.currentVideoPts {
		p.currentVideoPts = pts
		p.lastPtsUpdate = now

		// 定期触发进度更新回调（避免过于频繁）
		if p.OnProgressUpdate != nil && now.Sub(p.lastProgressUpdate) >= p.ProgressUpdatePeriod {
			p.lastProgressUpdate = now
			p.OnProgressUpdate()
		}
	}
}

// GetElapsedSeconds 获取已播放的时长（秒），基于视频 PTS
// PTS 时间戳单位是 90kHz（MPEG标准时钟频率）
func (p *PSReceiver) GetElapsedSeconds() float64 {
	if !p.hasFirstPts {
		return 0
	}
	// 计算 PTS 差值（处理回绕）
	var diff uint64
	if p.currentVideoPts >= p.firstVideoPts {
		diff = p.currentVideoPts - p.firstVideoPts
	} else {
		// 33位PTS回绕（虽然极少发生）
		diff = (0x1FFFFFFFF - p.firstVideoPts) + p.currentVideoPts + 1
	}
	// 转换为秒：pts / 90000
	return float64(diff) / 90000.0
}

// IsPtsStable 检查视频 PTS 是否已经稳定（停止增长）
// 如果 PTS 超过 2 秒没有变化，认为已经稳定
func (p *PSReceiver) IsPtsStable() bool {
	if !p.hasFirstPts {
		return false
	}
	return time.Since(p.lastPtsUpdate) > 2*time.Second
}

// GetLastPtsUpdateTime 获取最后一次 PTS 更新的时间
func (p *PSReceiver) GetLastPtsUpdateTime() time.Time {
	return p.lastPtsUpdate
}

func (p *Receiver) Start() (err error) {
	// 如果已经启动过，直接返回
	if p.started {
		return nil
	}
	p.started = true

	var rtpReader *RTPPayloadReader
	switch p.StreamMode {
	case StreamModeTCPActive:
		// TCP主动模式不需要监听，直接返回
		p.Info("TCP-ACTIVE mode, no need to listen")
		addr := p.ListenAddr
		if !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
		if strings.HasPrefix(addr, ":") {
			p.Error("invalid address, missing IP", "addr", addr)
			return fmt.Errorf("invalid address %s, missing IP", addr)
		}
		p.Info("TCP-ACTIVE mode, connecting to device", "addr", addr)
		var conn net.Conn
		conn, err = net.Dial("tcp", addr)
		if err != nil {
			p.Error("connect to device failed", "err", err)
			return err
		}
		p.OnStop(conn.Close)
		rtpReader = NewRTPPayloadReader(NewRTPTCPReader(conn))
		p.rtpReader = rtpReader
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeTCPPassive:
		var conn io.ReadCloser
		if p.SinglePort == nil {
			p.Info("start new listener", "addr", p.ListenAddr)
			p.Listener, err = net.Listen("tcp4", p.ListenAddr)
			if err != nil {
				p.Error("start listen", "err", err)
				return errors.New("start listen,err" + err.Error())
			}
			p.OnStop(p.Listener.Close)
			return
		} else {
			conn = p.SinglePort
		}
		p.OnStop(conn.Close)
		rtpReader = NewRTPPayloadReader(NewRTPTCPReader(conn))
		p.rtpReader = rtpReader
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeUDP:
		var conn io.ReadCloser
		if p.SinglePort == nil {
			var udpAddr *net.UDPAddr
			udpAddr, err = net.ResolveUDPAddr("udp4", p.ListenAddr)
			if err != nil {
				return
			}
			conn, err = net.ListenUDP("udp4", udpAddr)
			if err != nil {
				return
			}
		} else {
			conn = p.SinglePort
		}
		p.OnStop(conn.Close)
		rtpReader = NewRTPPayloadReader(NewRTPUDPReader(conn))
		p.rtpReader = rtpReader
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeManual:
		p.Info("进入 StreamModeManual 分支")
		p.RTPMouth = make(chan []byte)
		rtpReader = NewRTPPayloadReader((RTPChanReader)(p.RTPMouth))
		p.rtpReader = rtpReader
		p.BufReader = util.NewBufReader(rtpReader)
	default:
		p.Error("未知的 StreamMode", "StreamMode", p.StreamMode)
		return fmt.Errorf("unknown StreamMode: %s", p.StreamMode)
	}
	p.Using(rtpReader, p.BufReader)
	return
}

func (p *Receiver) Run() error {
	if p.Listener != nil {
		// 循环Accept，支持SSRC过滤时拒绝不匹配的连接
		for {
			conn, err := p.Accept()
			if err != nil {
				return err
			}

			// 如果设置了ExpectedSSRC，验证第一个RTP包的SSRC
			if p.ExpectedSSRC != 0 {
				// TCP模式：RFC 4571格式，前2字节是长度，然后是RTP包
				var buffer [14]byte // 2字节长度 + 12字节RTP header
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				_, err := io.ReadFull(conn, buffer[:])
				conn.SetReadDeadline(time.Time{})

				if err != nil {
					p.Warn("failed to read RTP header for SSRC validation", "err", err, "remote", conn.RemoteAddr())
					conn.Close()
					continue
				}

				// 解析SSRC（跳过2字节长度前缀，RTP header的字节8-11）
				ssrc := binary.BigEndian.Uint32(buffer[10:14])

				if ssrc != p.ExpectedSSRC {
					p.Warn("reject connection with wrong SSRC",
						"expected", p.ExpectedSSRC,
						"actual", ssrc,
						"remote", conn.RemoteAddr())
					conn.Close()
					continue
				}

				p.Info("accept connection with correct SSRC", "ssrc", ssrc, "remote", conn.RemoteAddr())

				// 创建带缓冲的连接，包含已读取的数据（2字节长度+12字节RTP header）
				conn = &BufferedConn{
					Conn:   conn,
					buffer: buffer[:],
				}
			}

			p.OnStop(conn.Close)
			rtpReader := NewRTPPayloadReader(NewRTPTCPReader(conn))
			p.rtpReader = rtpReader
			p.BufReader = util.NewBufReader(rtpReader)
			return nil
		}
	}
	return nil
}

// BufferedConn 包装已读取的数据，用于SSRC验证后重新读取RTP header
type BufferedConn struct {
	net.Conn
	buffer []byte
	offset int
}

func (bc *BufferedConn) Read(p []byte) (n int, err error) {
	// 先读取缓冲区中的数据
	if bc.offset < len(bc.buffer) {
		n = copy(p, bc.buffer[bc.offset:])
		bc.offset += n
		return n, nil
	}
	// 缓冲区读完后，从底层连接读取
	return bc.Conn.Read(p)
}
