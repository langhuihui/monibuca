package rtp

import (
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
	StreamMode StreamMode
	RTPMouth   chan []byte
	SinglePort io.ReadCloser
	rtpReader  *RTPPayloadReader // 保存 RTP 读取器引用
}

type PSReceiver struct {
	Receiver
	mpegps.MpegPsDemuxer
	firstRtpTimestamp    uint32        // 第一个 RTP 包的时间戳
	currentRtpTimestamp  uint32        // 当前 RTP 包的时间戳
	hasFirstTimestamp    bool          // 是否已记录第一个时间戳
	lastTimestampUpdate  time.Time     // 最后一次时间戳更新的时间
	OnProgressUpdate     func()        // 进度更新回调（可选，导出供外部使用）
	lastProgressUpdate   time.Time     // 最后一次进度更新时间
	ProgressUpdatePeriod time.Duration // 进度更新周期（默认1秒，导出供外部配置）
}

func (p *PSReceiver) Start() error {
	err := p.Receiver.Start()
	if err == nil {
		p.Using(p.Publisher)
		// 设置 RTP 时间戳更新回调
		if p.rtpReader != nil {
			p.rtpReader.onTimestampUpdate = p.UpdateRtpTimestamp
		}
	}
	return err
}

func (p *PSReceiver) Run() error {
	p.MpegPsDemuxer.Allocator = gomem.NewScalableMemoryAllocator(1 << gomem.MinPowerOf2)
	p.Using(p.MpegPsDemuxer.Allocator)
	return p.MpegPsDemuxer.Feed(p.BufReader)
}

// UpdateRtpTimestamp 更新 RTP 时间戳（从 RTP 包中调用）
func (p *PSReceiver) UpdateRtpTimestamp(timestamp uint32) {
	now := time.Now()
	
	if !p.hasFirstTimestamp {
		p.firstRtpTimestamp = timestamp
		p.hasFirstTimestamp = true
		p.lastTimestampUpdate = now
		p.lastProgressUpdate = now
		// 默认进度更新周期为1秒
		if p.ProgressUpdatePeriod == 0 {
			p.ProgressUpdatePeriod = time.Second
		}
	}
	
	// 检测时间戳是否变化
	if timestamp != p.currentRtpTimestamp {
		p.currentRtpTimestamp = timestamp
		p.lastTimestampUpdate = now
		
		// 定期触发进度更新回调（避免过于频繁）
		if p.OnProgressUpdate != nil && now.Sub(p.lastProgressUpdate) >= p.ProgressUpdatePeriod {
			p.lastProgressUpdate = now
			p.OnProgressUpdate()
		}
	}
}

// GetElapsedSeconds 获取已播放的时长（秒），基于 RTP 时间戳
// RTP 时间戳单位是 90kHz（视频标准时钟频率）
func (p *PSReceiver) GetElapsedSeconds() float64 {
	if !p.hasFirstTimestamp {
		return 0
	}
	// 计算时间戳差值（处理回绕）
	var diff uint32
	if p.currentRtpTimestamp >= p.firstRtpTimestamp {
		diff = p.currentRtpTimestamp - p.firstRtpTimestamp
	} else {
		// 32位回绕
		diff = (0xFFFFFFFF - p.firstRtpTimestamp) + p.currentRtpTimestamp + 1
	}
	// 转换为秒：timestamp / 90000
	return float64(diff) / 90000.0
}

// IsTimestampStable 检查 RTP 时间戳是否已经稳定（停止增长）
// 如果时间戳超过 2 秒没有变化，认为已经稳定
func (p *PSReceiver) IsTimestampStable() bool {
	if !p.hasFirstTimestamp {
		return false
	}
	return time.Since(p.lastTimestampUpdate) > 2*time.Second
}

func (p *Receiver) Start() (err error) {
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
			conn, err = p.Accept()
		} else {
			conn = p.SinglePort
		}
		if err != nil {
			p.Error("accept", "err", err)
			return err
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
		p.RTPMouth = make(chan []byte)
		rtpReader = NewRTPPayloadReader((RTPChanReader)(p.RTPMouth))
		p.rtpReader = rtpReader
		p.BufReader = util.NewBufReader(rtpReader)
	}
	p.Using(rtpReader, p.BufReader)
	return
}
