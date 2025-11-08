package rtp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pion/rtp"
	"m7s.live/v5/pkg/util"
)

// ConnectionConfig 连接配置
type ConnectionConfig struct {
	IP   string
	Port uint16
	Mode StreamMode
	SSRC uint32 // RTP SSRC
}

// ForwardConfig 转发配置
type ForwardConfig struct {
	Source ConnectionConfig
	Target ConnectionConfig
	Relay  bool
}

// Forwarder 转发器
type Forwarder struct {
	config         *ForwardConfig
	source         net.Conn
	target         net.Conn
	sourceListener net.Listener // 保存source的listener，用于cleanup时关闭
	targetListener net.Listener // 保存target的listener，用于cleanup时关闭
}

// NewForwarder 创建新的转发器
func NewForwarder(config *ForwardConfig) *Forwarder {
	return &Forwarder{
		config: config,
	}
}

// establishSourceConnection 建立源连接
func (f *Forwarder) establishSourceConnection(config ConnectionConfig) (net.Conn, error) {
	switch config.Mode {
	case StreamModeTCPActive:
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		netConn, err := dialer.Dial("tcp", fmt.Sprintf("%s:%d", config.IP, config.Port))
		if err != nil {
			return nil, fmt.Errorf("connect failed: %v", err)
		}
		return netConn, nil

	case StreamModeTCPPassive:
		addr := fmt.Sprintf(":%d", config.Port)
		fmt.Printf("[Forwarder] TCP-PASSIVE: 开始监听端口 %s\n", addr)
		listener, err := net.Listen("tcp4", addr)
		if err != nil {
			fmt.Printf("[Forwarder] TCP-PASSIVE: 监听失败 %s, err=%v\n", addr, err)
			return nil, fmt.Errorf("listen failed: %v", err)
		}
		fmt.Printf("[Forwarder] TCP-PASSIVE: 监听成功 %s, listener=%p\n", addr, listener)

		// 保存listener，用于cleanup时关闭
		f.sourceListener = listener

		// Set timeout for accepting connections
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(30 * time.Second))
		}

		fmt.Printf("[Forwarder] TCP-PASSIVE: 等待连接 %s\n", addr)
		netConn, err := listener.Accept()
		if err != nil {
			fmt.Printf("[Forwarder] TCP-PASSIVE: Accept失败 %s, err=%v, 关闭listener\n", addr, err)
			listener.Close()
			f.sourceListener = nil
			return nil, fmt.Errorf("accept failed: %v", err)
		}
		fmt.Printf("[Forwarder] TCP-PASSIVE: Accept成功 %s, conn=%p, listener=%p 已保存到f.sourceListener\n", addr, netConn, listener)

		return netConn, nil

	case StreamModeUDP:
		// Source UDP - listen
		udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", config.Port))
		if err != nil {
			return nil, fmt.Errorf("resolve UDP address failed: %v", err)
		}
		netConn, err := net.ListenUDP("udp4", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("UDP listen failed: %v", err)
		}
		return netConn, nil
	}

	return nil, fmt.Errorf("unsupported mode: %s", config.Mode)
}

// establishTargetConnection 建立目标连接
func (f *Forwarder) establishTargetConnection(config ConnectionConfig) (net.Conn, error) {
	switch config.Mode {
	case StreamModeTCPPassive:
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		netConn, err := dialer.Dial("tcp", fmt.Sprintf("%s:%d", config.IP, config.Port))
		if err != nil {
			return nil, fmt.Errorf("connect failed: %v", err)
		}
		return netConn, nil

	case StreamModeTCPActive:
		listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", config.Port))
		if err != nil {
			return nil, fmt.Errorf("listen failed: %v", err)
		}

		// 保存listener，用于cleanup时关闭
		f.targetListener = listener

		// Set timeout for accepting connections
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(30 * time.Second))
		}

		netConn, err := listener.Accept()
		if err != nil {
			listener.Close()
			f.targetListener = nil
			return nil, fmt.Errorf("accept failed: %v", err)
		}

		return netConn, nil

	case StreamModeUDP:
		// Target UDP - dial
		netConn, err := net.DialUDP("udp", nil, &net.UDPAddr{
			IP:   net.ParseIP(config.IP),
			Port: int(config.Port),
		})
		if err != nil {
			return nil, fmt.Errorf("UDP dial failed: %v", err)
		}
		return netConn, nil
	}

	return nil, fmt.Errorf("unsupported mode: %s", config.Mode)
}

// setupConnections 建立源和目标连接
func (f *Forwarder) setupConnections() error {
	var err error

	// 建立源连接
	f.source, err = f.establishSourceConnection(f.config.Source)
	if err != nil {
		return fmt.Errorf("source connection failed: %v", err)
	}

	// 建立目标连接
	f.target, err = f.establishTargetConnection(f.config.Target)
	if err != nil {
		return fmt.Errorf("target connection failed: %v", err)
	}

	return nil
}

// cleanup 清理连接
func (f *Forwarder) cleanup() {
	fmt.Printf("[Forwarder] cleanup: 开始清理, source=%p, target=%p, sourceListener=%p, targetListener=%p\n",
		f.source, f.target, f.sourceListener, f.targetListener)

	// 先关闭连接
	if f.source != nil {
		fmt.Printf("[Forwarder] cleanup: 关闭source连接 %p\n", f.source)
		f.source.Close()
	}
	if f.target != nil {
		fmt.Printf("[Forwarder] cleanup: 关闭target连接 %p\n", f.target)
		f.target.Close()
	}

	// 再关闭listener（重要！释放端口）
	if f.sourceListener != nil {
		fmt.Printf("[Forwarder] cleanup: ✅ 关闭sourceListener %p，释放端口\n", f.sourceListener)
		f.sourceListener.Close()
		f.sourceListener = nil
	}
	if f.targetListener != nil {
		fmt.Printf("[Forwarder] cleanup: ✅ 关闭targetListener %p，释放端口\n", f.targetListener)
		f.targetListener.Close()
		f.targetListener = nil
	}

	fmt.Printf("[Forwarder] cleanup: 清理完成\n")
}

// createRTPReader 创建RTP读取器
func (f *Forwarder) createRTPReader() IRTPReader {
	switch f.config.Source.Mode {
	case StreamModeUDP:
		return NewRTPUDPReader(f.source)
	case StreamModeTCPActive, StreamModeTCPPassive:
		return NewRTPTCPReader(f.source)
	default:
		return nil
	}
}

// createRTPWriter 创建RTP写入器
func (f *Forwarder) createRTPWriter() RTPWriter {
	return NewRTPWriter(f.target, f.config.Target.Mode)
}

// RTPWriter RTP写入器接口
type RTPWriter interface {
	WritePacket(packet *rtp.Packet) error
	WriteRaw(data []byte) error
}

// rtpWriter RTP写入器实现
type rtpWriter struct {
	writer     io.Writer
	mode       StreamMode
	header     []byte
	sendBuffer util.Buffer // 可复用的发送缓冲区
}

// NewRTPWriter 创建RTP写入器
func NewRTPWriter(writer io.Writer, mode StreamMode) RTPWriter {
	return &rtpWriter{
		writer:     writer,
		mode:       mode,
		header:     make([]byte, 2),
		sendBuffer: util.Buffer{}, // 初始化可复用缓冲区
	}
}

// WritePacket 写入RTP包
func (w *rtpWriter) WritePacket(packet *rtp.Packet) error {
	// 复用sendBuffer，避免重复创建
	w.sendBuffer.Reset()
	w.sendBuffer.Malloc(packet.MarshalSize())
	_, err := packet.MarshalTo(w.sendBuffer)
	if err != nil {
		return fmt.Errorf("marshal RTP packet failed: %v", err)
	}

	return w.WriteRaw(w.sendBuffer)
}

// WriteRaw 写入原始数据
func (w *rtpWriter) WriteRaw(data []byte) error {
	if w.mode == StreamModeUDP {
		_, err := w.writer.Write(data)
		return err
	} else {
		// TCP模式需要添加长度头
		binary.BigEndian.PutUint16(w.header, uint16(len(data)))
		_, err := w.writer.Write(w.header)
		if err != nil {
			return err
		}
		_, err = w.writer.Write(data)
		return err
	}
}

// RelayProcessor 中继处理器
type RelayProcessor struct {
	reader     io.Reader
	writer     io.Writer
	sourceMode StreamMode
	targetMode StreamMode
	buffer     []byte // 可复用的缓冲区
	header     []byte // 可复用的头部缓冲区
}

// NewRelayProcessor 创建中继处理器
func NewRelayProcessor(reader io.Reader, writer io.Writer, sourceMode, targetMode StreamMode) *RelayProcessor {
	return &RelayProcessor{
		reader:     reader,
		writer:     writer,
		sourceMode: sourceMode,
		targetMode: targetMode,
		buffer:     make([]byte, 1460), // 初始化可复用缓冲区
		header:     make([]byte, 2),    // 初始化可复用头部缓冲区
	}
}

// Process 处理中继
func (p *RelayProcessor) Process(ctx context.Context) error {
	if p.sourceMode == p.targetMode {
		// 相同模式直接复制
		_, err := io.Copy(p.writer, p.reader)
		return err
	}

	// 不同模式需要转换
	if p.sourceMode == StreamModeUDP && (p.targetMode == StreamModeTCPActive || p.targetMode == StreamModeTCPPassive) {
		// UDP to TCP
		return p.processUDPToTCP(ctx)
	} else if (p.sourceMode == StreamModeTCPActive || p.sourceMode == StreamModeTCPPassive) && p.targetMode == StreamModeUDP {
		// TCP to UDP
		return p.processTCPToUDP(ctx)
	}

	return fmt.Errorf("unsupported mode combination")
}

// processUDPToTCP UDP转TCP
func (p *RelayProcessor) processUDPToTCP(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := p.reader.Read(p.buffer)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		// 添加2字节长度头
		binary.BigEndian.PutUint16(p.header, uint16(n))
		_, err = p.writer.Write(p.header)
		if err != nil {
			return err
		}

		_, err = p.writer.Write(p.buffer[:n])
		if err != nil {
			return err
		}
	}
}

// processTCPToUDP TCP转UDP
func (p *RelayProcessor) processTCPToUDP(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 读取2字节长度头
		_, err := io.ReadFull(p.reader, p.header)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		// 获取包长度
		packetLength := binary.BigEndian.Uint16(p.header)

		// 如果包长度超过缓冲区大小，需要动态分配
		if packetLength > uint16(len(p.buffer)) {
			packetData := make([]byte, packetLength)
			_, err = io.ReadFull(p.reader, packetData)
			if err != nil {
				return err
			}
			_, err = p.writer.Write(packetData)
		} else {
			// 使用可复用缓冲区
			_, err = io.ReadFull(p.reader, p.buffer[:packetLength])
			if err != nil {
				return err
			}
			_, err = p.writer.Write(p.buffer[:packetLength])
		}

		if err != nil {
			return err
		}
	}
}

// RTPProcessor RTP处理器
type RTPProcessor struct {
	reader     IRTPReader
	writer     RTPWriter
	config     *ForwardConfig
	sendBuffer util.Buffer // 可复用的发送缓冲区
}

// NewRTPProcessor 创建RTP处理器
func NewRTPProcessor(reader IRTPReader, writer RTPWriter, config *ForwardConfig) *RTPProcessor {
	return &RTPProcessor{
		reader:     reader,
		writer:     writer,
		config:     config,
		sendBuffer: util.Buffer{}, // 初始化可复用缓冲区
	}
}

// Process 处理RTP包
func (p *RTPProcessor) Process(ctx context.Context) error {
	var packet rtp.Packet
	var sequenceNumber uint16

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := p.reader.Read(&packet)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read RTP packet failed: %v", err)
		}

		// 检查源SSRC过滤
		if p.config.Source.SSRC != 0 && packet.SSRC != p.config.Source.SSRC {
			continue
		}

		// 保存原始序列号用于分片包
		sequenceNumber = packet.SequenceNumber

		// 检查是否需要分片
		if len(packet.Payload) > (1460 - packet.MarshalSize()) {
			err = p.processFragmentedPacket(&packet, sequenceNumber)
		} else {
			err = p.processSinglePacket(&packet)
		}

		if err != nil {
			return err
		}
	}
}

// processSinglePacket 处理单个包
func (p *RTPProcessor) processSinglePacket(packet *rtp.Packet) error {
	if p.config.Target.SSRC != 0 {
		packet.SSRC = p.config.Target.SSRC
	}

	return p.writer.WritePacket(packet)
}

// processFragmentedPacket 处理分片包
func (p *RTPProcessor) processFragmentedPacket(packet *rtp.Packet, sequenceNumber uint16) error {
	maxPayloadSize := 1460 - 12 // RTP头通常是12字节
	payload := packet.Payload

	// 标记第一个包
	marker := packet.Marker
	packet.Marker = false

	for i := 0; i < len(payload); i += int(maxPayloadSize) {
		end := i + int(maxPayloadSize)
		if end > len(payload) {
			end = len(payload)
			// 最后一个分片，恢复原始标记
			packet.Marker = marker
		}

		// 创建包含分片的新包
		fragmentPacket := *packet
		if p.config.Target.SSRC != 0 {
			fragmentPacket.SSRC = p.config.Target.SSRC
		}
		fragmentPacket.SequenceNumber = sequenceNumber
		sequenceNumber++
		fragmentPacket.Payload = payload[i:end]

		err := p.writer.WritePacket(&fragmentPacket)
		if err != nil {
			return fmt.Errorf("write RTP fragment failed: %v", err)
		}
	}

	return nil
}

// Forward 执行转发
func (f *Forwarder) Forward(ctx context.Context) error {
	fmt.Printf("[Forwarder] Forward: 开始, source=%s:%d mode=%s, target=%s:%d mode=%s\n",
		f.config.Source.IP, f.config.Source.Port, f.config.Source.Mode,
		f.config.Target.IP, f.config.Target.Port, f.config.Target.Mode)

	// 建立连接
	err := f.setupConnections()
	if err != nil {
		fmt.Printf("[Forwarder] Forward: setupConnections失败, err=%v\n", err)
		return err
	}
	fmt.Printf("[Forwarder] Forward: setupConnections成功, source=%p, target=%p\n", f.source, f.target)
	defer func() {
		fmt.Printf("[Forwarder] Forward: 准备执行cleanup (defer)\n")
		f.cleanup()
	}()

	// 检查是否为中继模式
	if f.config.Relay {
		fmt.Printf("[Forwarder] Forward: 使用中继模式\n")
		processor := NewRelayProcessor(f.source, f.target, f.config.Source.Mode, f.config.Target.Mode)
		err := processor.Process(ctx)
		fmt.Printf("[Forwarder] Forward: 中继模式结束, err=%v\n", err)
		return err
	}

	// RTP处理模式
	fmt.Printf("[Forwarder] Forward: 使用RTP处理模式\n")
	reader := f.createRTPReader()
	writer := f.createRTPWriter()
	processor := NewRTPProcessor(reader, writer, f.config)

	err = processor.Process(ctx)
	fmt.Printf("[Forwarder] Forward: RTP处理模式结束, err=%v\n", err)
	return err
}
