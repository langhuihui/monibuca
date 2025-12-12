package plugin_gb28181pro

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/rtp"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

// RTP 包大小常量
const (
	MinRTPPayloadSize    = 160  // 最小 RTP 负载大小 (20ms @ 8000Hz)
	DefaultRTPPayloadMax = 1200 // 默认最大 RTP 负载大小
	RTPHeaderOverhead    = 40   // IP(20) + UDP(8) + RTP(12) 头开销
)

// MaxRTPPayloadSize 动态计算的最大 RTP 负载大小
var MaxRTPPayloadSize = DefaultRTPPayloadMax

// BroadcastSession 语音广播会话
type BroadcastSession struct {
	Device         *Device
	Channel        *Channel
	gb             *GB28181Plugin
	Session        *sipgo.DialogClientSession
	RTPPort        int
	RTPPeerIP      string
	RTPPeerPort    int
	SSRC           string
	CallID         string
	PayloadType    uint8
	TranscodePCMA  bool
	inviteCh       chan struct{}
	audioChan      chan []byte // 音频数据通道，用于接收 WebSocket 数据
	ready          bool        // 标记会话是否已准备好（ACK 已收到）
	isTCP          bool        // 是否使用 TCP 传输
	rtpConn        *net.UDPConn
	tcpListener    net.Listener // TCP 监听器
	tcpConn        net.Conn     // TCP 连接
	sequenceNumber uint16
	timestamp      uint32
	audioBuffer    []byte
}

// 全局会话管理
var BroadcastSessions util.Collection[string, *BroadcastSession]

// GetKey 实现 util.Collection 接口
func (bs *BroadcastSession) GetKey() string {
	if bs.Channel != nil {
		return bs.Channel.ChannelId
	}
	return ""
}

// DetectMTU 检测网络接口的 MTU 并计算合适的 RTP 负载大小
func DetectMTU(ipAddress string) int {
	interfaces, err := net.Interfaces()
	if err != nil {
		return DefaultRTPPayloadMax
	}

	minMTU := 1500
	foundSpecific := false

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		if ipAddress != "" {
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && ip.String() == ipAddress {
					foundSpecific = true
					minMTU = iface.MTU
					break
				}
			}
			if foundSpecific {
				break
			}
		} else {
			if iface.MTU > 0 && iface.MTU < minMTU {
				minMTU = iface.MTU
			}
		}
	}

	if ipAddress != "" && !foundSpecific {
		return DetectMTU("")
	}

	payloadSize := minMTU - RTPHeaderOverhead
	if payloadSize < MinRTPPayloadSize {
		payloadSize = MinRTPPayloadSize
	}
	if payloadSize > 1400 {
		payloadSize = 1400
	}
	return payloadSize
}

// InitRTPPayloadSize 初始化 RTP 负载大小
func InitRTPPayloadSize() {
	MaxRTPPayloadSize = DetectMTU("")
}

// StartBroadcast 启动广播会话
func (d *Device) StartBroadcast(channelId string) (*BroadcastSession, error) {
	// 查找通道
	var channel *Channel
	d.channels.Range(func(c *Channel) bool {
		if c.ChannelId == channelId {
			channel = c
			return false
		}
		return true
	})

	if channel == nil {
		return nil, fmt.Errorf("channel not found: %s", channelId)
	}

	// 创建会话
	bs := &BroadcastSession{
		Device:    d,
		Channel:   channel,
		gb:        d.plugin,
		inviteCh:  make(chan struct{}),
		audioChan: make(chan []byte, 100), // 缓冲 100 个音频包
		ready:     false,
	}

	// 生成 SSRC
	bs.SSRC = bs.generateSSRC()

	// 准备 RTP 连接
	if err := bs.prepareRTPConn(); err != nil {
		return nil, fmt.Errorf("prepare rtp conn failed: %v", err)
	}

	// 缓存会话
	BroadcastSessions.Set(bs)

	// 发送 SIP MESSAGE
	sourceID := d.plugin.Serial
	if sourceID == "" {
		sourceID = d.DeviceId
	}
	if err := d.SendBroadcast(sourceID, channelId); err != nil {
		BroadcastSessions.Remove(bs)
		return nil, err
	}

	return bs, nil
}

// generateSSRC 生成 SSRC
func (bs *BroadcastSession) generateSSRC() string {
	return fmt.Sprintf("%010d", bs.Device.CreateSSRC(bs.Device.plugin.Serial))
}

// prepareRTPConn 准备 RTP 连接
func (bs *BroadcastSession) prepareRTPConn() error {
	// 从端口位图分配端口
	var port uint16
	var ok bool
	if bs.Device.plugin.MediaPort.Valid() && bs.Device.plugin.MediaPort.Size() > 0 {
		port, ok = bs.Device.plugin.udpPB.Allocate()
		if !ok {
			return fmt.Errorf("no available port")
		}
	} else {
		// 单端口模式
		port = bs.Device.plugin.udpPort
	}

	localAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		if bs.Device.plugin.MediaPort.Valid() && bs.Device.plugin.MediaPort.Size() > 0 {
			bs.Device.plugin.udpPB.Release(port)
		}
		return err
	}

	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		if bs.Device.plugin.MediaPort.Valid() && bs.Device.plugin.MediaPort.Size() > 0 {
			bs.Device.plugin.udpPB.Release(port)
		}
		return err
	}

	bs.rtpConn = conn
	bs.RTPPort = int(port)
	return nil
}

// prepareTCPListener 准备 TCP 监听器（用于 TCP/RTP）
func (bs *BroadcastSession) prepareTCPListener() error {
	// 从端口位图分配端口
	var port uint16
	var ok bool
	if bs.Device.plugin.MediaPort.Valid() && bs.Device.plugin.MediaPort.Size() > 0 {
		port, ok = bs.Device.plugin.tcpPB.Allocate()
		if !ok {
			return fmt.Errorf("no available tcp port")
		}
	} else {
		// 单端口模式
		port = bs.Device.plugin.tcpPort
	}

	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		if bs.Device.plugin.MediaPort.Valid() && bs.Device.plugin.MediaPort.Size() > 0 {
			bs.Device.plugin.tcpPB.Release(port)
		}
		return fmt.Errorf("listen failed: %v", err)
	}

	bs.tcpListener = listener
	bs.RTPPort = int(port)
	bs.gb.Info("prepareTCPListener: TCP listener created", "port", port)
	return nil
}

// WaitInvite 等待设备 INVITE
func (bs *BroadcastSession) WaitInvite(ctx context.Context) error {
	if bs.inviteCh == nil {
		return fmt.Errorf("invite channel not initialized")
	}

	select {
	case <-bs.inviteCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// notifyInvite 通知已收到 INVITE
func (bs *BroadcastSession) notifyInvite() {
	if bs.inviteCh != nil {
		close(bs.inviteCh)
		bs.inviteCh = nil
	}
}

// startAudioSender 启动音频发送 goroutine
func (bs *BroadcastSession) startAudioSender() {
	bs.gb.Info("startAudioSender: goroutine started", "channelId", bs.Channel.ChannelId, "isTCP", bs.isTCP, "time", time.Now().Format("15:04:05.000"))

	// 如果是 TCP 模式，先等待设备连接
	if bs.isTCP && bs.tcpListener != nil {
		bs.gb.Info("startAudioSender: waiting for TCP connection...")

		// 设置超时
		if tcpListener, ok := bs.tcpListener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(30 * time.Second))
		}

		conn, err := bs.tcpListener.Accept()
		if err != nil {
			bs.gb.Error("startAudioSender: TCP accept failed", "error", err)
			return
		}

		bs.tcpConn = conn
		bs.gb.Info("startAudioSender: TCP connection established", "remoteAddr", conn.RemoteAddr().String(), "time", time.Now().Format("15:04:05.000"))
	}

	for audioData := range bs.audioChan {
		receiveTime := time.Now()
		bs.gb.Info("startAudioSender: received audio from channel",
			"dataLen", len(audioData),
			"ready", bs.ready,
			"time", receiveTime.Format("15:04:05.000"))

		// 等待 ACK（ready 标志）
		waitStart := time.Now()
		for !bs.ready {
			time.Sleep(50 * time.Millisecond)
		}
		waitDuration := time.Since(waitStart)
		if waitDuration > 0 {
			bs.gb.Info("startAudioSender: waited for ready", "duration", waitDuration)
		}

		// 发送音频数据
		sendStart := time.Now()
		if err := bs.sendAudioDataInternal(audioData); err != nil {
			bs.gb.Error("startAudioSender: send failed", "error", err, "time", time.Now().Format("15:04:05.000"))
		} else {
			sendDuration := time.Since(sendStart)
			bs.gb.Info("startAudioSender: sent RTP packet",
				"dataLen", len(audioData),
				"sendDuration", sendDuration,
				"totalDuration", time.Since(receiveTime),
				"time", time.Now().Format("15:04:05.000"))
		}
	}

	bs.gb.Info("startAudioSender: goroutine stopped", "channelId", bs.Channel.ChannelId, "time", time.Now().Format("15:04:05.000"))
}

// SendAudioData 接收来自 WebSocket 的音频数据，放入通道
func (bs *BroadcastSession) SendAudioData(data []byte) error {
	queueTime := time.Now()
	select {
	case bs.audioChan <- data:
		bs.gb.Debug("SendAudioData: queued to channel",
			"dataLen", len(data),
			"queueLen", len(bs.audioChan),
			"time", queueTime.Format("15:04:05.000"))
		return nil
	default:
		bs.gb.Error("SendAudioData: channel full",
			"dataLen", len(data),
			"time", queueTime.Format("15:04:05.000"))
		return fmt.Errorf("audio channel full")
	}
}

// sendAudioDataInternal 实际发送音频数据到设备
func (bs *BroadcastSession) sendAudioDataInternal(data []byte) error {
	if bs.rtpConn == nil {
		return fmt.Errorf("RTP connection not established")
	}

	if bs.RTPPeerIP == "" || bs.RTPPeerPort == 0 {
		return fmt.Errorf("RTP peer information not available")
	}

	// 解析 SSRC
	ssrcUint64, err := strconv.ParseUint(bs.SSRC, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid SSRC: %w", err)
	}

	// 解析远程地址
	remoteAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bs.RTPPeerIP, bs.RTPPeerPort))
	if err != nil {
		return fmt.Errorf("failed to resolve remote UDP address: %w", err)
	}

	// 追加到缓冲区
	bs.audioBuffer = append(bs.audioBuffer, data...)

	// 按 MaxRTPPayloadSize 分块发送
	for len(bs.audioBuffer) >= MaxRTPPayloadSize {
		chunk := bs.audioBuffer[:MaxRTPPayloadSize]
		bs.audioBuffer = bs.audioBuffer[MaxRTPPayloadSize:]

		if err := bs.sendRTPPacket(chunk, ssrcUint64, remoteAddr); err != nil {
			return err
		}
	}

	return nil
}

// sendRTPPacket 发送单个 RTP 包
func (bs *BroadcastSession) sendRTPPacket(data []byte, ssrc uint64, remoteAddr *net.UDPAddr) error {
	bs.sequenceNumber++

	payload := data
	if bs.TranscodePCMA {
		payload = make([]byte, len(data))
		for i, b := range data {
			payload[i] = ALawToULawTable[b]
		}
	}

	rtpPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			Padding:        false,
			Extension:      false,
			Marker:         false,
			PayloadType:    bs.PayloadType,
			SequenceNumber: bs.sequenceNumber,
			Timestamp:      bs.timestamp,
			SSRC:           uint32(ssrc),
		},
		Payload: payload,
	}

	bs.timestamp += uint32(len(data))

	rtpData, err := rtpPacket.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal RTP packet: %w", err)
	}

	if bs.isTCP {
		// TCP 模式：添加 2 字节长度头（RFC 4571）
		if bs.tcpConn == nil {
			return fmt.Errorf("TCP connection not established")
		}

		length := uint16(len(rtpData))
		lengthBytes := []byte{byte(length >> 8), byte(length & 0xFF)}

		// 先发送长度
		if _, err := bs.tcpConn.Write(lengthBytes); err != nil {
			return fmt.Errorf("failed to send RTP length: %w", err)
		}

		// 再发送 RTP 数据
		if _, err := bs.tcpConn.Write(rtpData); err != nil {
			return fmt.Errorf("failed to send RTP data: %w", err)
		}
	} else {
		// UDP 模式
		if _, err := bs.rtpConn.WriteTo(rtpData, remoteAddr); err != nil {
			return fmt.Errorf("failed to send audio data: %w", err)
		}
	}

	return nil
}

// FlushAudioBuffer 刷新音频缓冲区
func (bs *BroadcastSession) FlushAudioBuffer() error {
	if len(bs.audioBuffer) == 0 {
		return nil
	}

	if bs.rtpConn == nil || bs.RTPPeerIP == "" || bs.RTPPeerPort == 0 {
		return nil
	}

	ssrcUint64, err := strconv.ParseUint(bs.SSRC, 10, 64)
	if err != nil {
		return err
	}

	remoteAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bs.RTPPeerIP, bs.RTPPeerPort))
	if err != nil {
		return err
	}

	if err := bs.sendRTPPacket(bs.audioBuffer, ssrcUint64, remoteAddr); err != nil {
		return err
	}

	bs.audioBuffer = nil
	return nil
}

// StopBroadcast 停止广播
func (bs *BroadcastSession) StopBroadcast() error {
	// 从会话列表移除
	BroadcastSessions.Remove(bs)

	// 关闭音频通道
	if bs.audioChan != nil {
		close(bs.audioChan)
		bs.audioChan = nil
	}

	// 刷新缓冲区
	_ = bs.FlushAudioBuffer()

	// 发送 SIP BYE
	if bs.Session != nil {
		bs.Session.Bye(context.Background())
	}

	// 关闭 TCP 连接和监听器
	if bs.tcpConn != nil {
		bs.tcpConn.Close()
		bs.tcpConn = nil
	}

	if bs.tcpListener != nil {
		port := uint16(bs.tcpListener.Addr().(*net.TCPAddr).Port)
		bs.tcpListener.Close()
		bs.tcpListener = nil

		if bs.gb.MediaPort.Valid() && bs.gb.MediaPort.Size() > 0 {
			// 延迟 30 秒释放端口，防止端口重用问题
			go func() {
				time.Sleep(30 * time.Second)
				bs.gb.tcpPB.Release(port)
				bs.gb.Debug("TCP port released", "port", port)
			}()
		}
	}

	// 关闭 RTP 连接并归还端口（延迟释放）
	if bs.rtpConn != nil {
		port := uint16(bs.rtpConn.LocalAddr().(*net.UDPAddr).Port)
		bs.rtpConn.Close()

		if bs.gb.MediaPort.Valid() && bs.gb.MediaPort.Size() > 0 {
			// 延迟 30 秒释放端口，防止端口重用问题
			go func() {
				time.Sleep(30 * time.Second)
				bs.gb.udpPB.Release(port)
				bs.gb.Debug("UDP port released", "port", port)
			}()
		}
	}

	return nil
}

// SendBroadcast 发送广播 MESSAGE
func (d *Device) SendBroadcast(sourceId, targetId string) error {
	recipient := d.Recipient
	recipient.User = targetId

	request := d.CreateRequest(sip.MESSAGE, recipient)

	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Notify>
<CmdType>Broadcast</CmdType>
<SN>%d</SN>
<SourceID>%s</SourceID>
<TargetID>%s</TargetID>
</Notify>`, d.SN, sourceId, targetId)

	request.SetBody([]byte(xmlContent))

	response, err := d.send(request)
	if err != nil {
		return fmt.Errorf("send broadcast notification failed: %v", err)
	}

	if response.StatusCode != 200 {
		return fmt.Errorf("broadcast notification failed with status: %d", response.StatusCode)
	}

	return nil
}

// OnBroadcastInvite 处理广播 INVITE
func (gb *GB28181Plugin) OnBroadcastInvite(req *sip.Request, tx sip.ServerTransaction) {
	inviteTime := time.Now()
	gb.Info("OnBroadcastInvite: received", "time", inviteTime.Format("15:04:05.000"))

	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnBroadcastInvite: invalid from header")
		return
	}

	deviceID := from.Address.User
	device, ok := gb.devices.Get(deviceID)
	if !ok {
		gb.Error("OnBroadcastInvite: device not found", "deviceId", deviceID)
		return
	}

	// 解析 SDP
	sdpBody := string(req.Body())
	isBroadcast := strings.Contains(sdpBody, "s=Play")
	if !isBroadcast {
		return
	}

	// 发送 100 Trying
	tryingResp := sip.NewResponseFromRequest(req, sip.StatusTrying, "Trying", nil)
	if err := tx.Respond(tryingResp); err != nil {
		gb.Error("OnBroadcastInvite: send trying failed", "error", err)
		return
	}

	// 解析 SDP
	inviteInfo, err := gb28181.DecodeSDP(req)
	if err != nil {
		gb.Error("OnBroadcastInvite: decode sdp failed", "error", err)
		tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "SDP Decode Failed", nil))
		return
	}

	// 选择本地 IP
	sdpIP := gb.MediaIP
	if sdpIP == "" {
		sdpIP = gb.SipIP
	}
	if sdpIP == "" {
		sdpIP = device.SipIp
	}

	// 查找广播会话
	broadcastSession, ok := BroadcastSessions.Find(func(bs *BroadcastSession) bool {
		return bs.Device != nil && bs.Device.DeviceId == deviceID
	})
	if !ok {
		gb.Warn("OnBroadcastInvite: session not found, device may not have started broadcast", "deviceId", deviceID)
		tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Session Not Found", nil))
		return
	}

	// 记录对端信息和 Call-ID
	broadcastSession.RTPPeerIP = inviteInfo.IP
	broadcastSession.RTPPeerPort = int(inviteInfo.Port)
	if inviteInfo.SSRC != 0 {
		broadcastSession.SSRC = fmt.Sprintf("%010d", inviteInfo.SSRC)
	}

	// 保存 Call-ID，用于后续 ACK/BYE 关联
	callID := req.CallID()
	if callID != nil {
		broadcastSession.CallID = callID.Value()
	}

	// 协商编码格式和传输模式：解析 SDP
	broadcastSession.PayloadType = 8 // 默认 PCMA
	broadcastSession.TranscodePCMA = false

	sdpBody = string(req.Body())
	hasPCMA := strings.Contains(sdpBody, "rtpmap:8 PCMA")
	hasPCMU := strings.Contains(sdpBody, "rtpmap:0 PCMU")
	isTCP := strings.Contains(sdpBody, "TCP/RTP/AVP") || strings.Contains(sdpBody, "RTP/AVP/TCP")

	if hasPCMA {
		broadcastSession.PayloadType = 8
		broadcastSession.TranscodePCMA = false
		gb.Info("OnBroadcastInvite: negotiated PCMA", "deviceId", deviceID)
	} else if hasPCMU {
		broadcastSession.PayloadType = 0
		broadcastSession.TranscodePCMA = true
		gb.Info("OnBroadcastInvite: negotiated PCMU (will transcode from PCMA)", "deviceId", deviceID)
	} else {
		gb.Warn("OnBroadcastInvite: no supported codec in SDP, using PCMA", "deviceId", deviceID)
	}

	broadcastSession.isTCP = isTCP

	var mediaPort int
	if isTCP {
		// TCP 模式：创建 TCP Listener，等待设备连接
		gb.Info("OnBroadcastInvite: using TCP/RTP", "deviceId", deviceID)
		if err := broadcastSession.prepareTCPListener(); err != nil {
			gb.Error("OnBroadcastInvite: prepare tcp listener failed", "error", err)
			tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Create TCP Listener Failed", nil))
			return
		}
		mediaPort = broadcastSession.tcpListener.Addr().(*net.TCPAddr).Port
	} else {
		// UDP 模式
		gb.Info("OnBroadcastInvite: using UDP/RTP", "deviceId", deviceID)
		if broadcastSession.rtpConn == nil {
			if err := broadcastSession.prepareRTPConn(); err != nil {
				gb.Error("OnBroadcastInvite: prepare rtp conn failed", "error", err)
				tx.Respond(sip.NewResponseFromRequest(req, sip.StatusServiceUnavailable, "Create RTP Failed", nil))
				return
			}
		}
		mediaPort = broadcastSession.rtpConn.LocalAddr().(*net.UDPAddr).Port
	}

	// 通知控制面
	broadcastSession.notifyInvite()

	// 启动音频发送 goroutine
	startTime := time.Now()
	go broadcastSession.startAudioSender()
	gb.Info("OnBroadcastInvite: audio sender goroutine launched",
		"deviceId", deviceID,
		"time", startTime.Format("15:04:05.000"))

	// 构造 200 OK
	sdpLines := []string{
		"v=0",
		fmt.Sprintf("o=%s 0 0 IN IP4 %s", gb.Serial, sdpIP),
		"s=Play",
		fmt.Sprintf("c=IN IP4 %s", sdpIP),
		"t=0 0",
	}

	// 根据传输模式添加媒体行
	if isTCP {
		sdpLines = append(sdpLines, fmt.Sprintf("m=audio %d TCP/RTP/AVP %d", mediaPort, broadcastSession.PayloadType))
		sdpLines = append(sdpLines, "a=sendrecv")
		//sdpLines = append(sdpLines, "a=setup:passive") // 我们是被动方，等待设备连接
		//sdpLines = append(sdpLines, "a=connection:new")
	} else {
		sdpLines = append(sdpLines, fmt.Sprintf("m=audio %d RTP/AVP %d", mediaPort, broadcastSession.PayloadType))
		sdpLines = append(sdpLines, "a=sendrecv")
	}
	if broadcastSession.PayloadType == 8 {
		sdpLines = append(sdpLines, "a=rtpmap:8 PCMA/8000")
	} else if broadcastSession.PayloadType == 0 {
		sdpLines = append(sdpLines, "a=rtpmap:0 PCMU/8000")
	}

	if broadcastSession.SSRC != "" {
		sdpLines = append(sdpLines, fmt.Sprintf("y=%s", broadcastSession.SSRC))
	}
	sdpLines = append(sdpLines, "f=v/2/0/0/0/0a/0/0/0")

	okResp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	contentType := sip.ContentTypeHeader("application/sdp")
	okResp.AppendHeader(&contentType)
	contactAddr := sip.Uri{
		User: gb.Serial,
		Host: device.MediaIp,
		Port: device.LocalPort,
	}
	contactHeader := sip.ContactHeader{Address: contactAddr}
	okResp.AppendHeader(&contactHeader)
	okResp.SetBody([]byte(strings.Join(sdpLines, "\r\n") + "\r\n"))

	if err := tx.Respond(okResp); err != nil {
		gb.Error("OnBroadcastInvite: send ok failed", "error", err)
		return
	}

	responseTime := time.Now()
	gb.Info("OnBroadcastInvite: 200 OK sent",
		"deviceId", deviceID,
		"peerIP", broadcastSession.RTPPeerIP,
		"peerPort", broadcastSession.RTPPeerPort,
		"localPort", mediaPort,
		"processingTime", responseTime.Sub(inviteTime),
		"time", responseTime.Format("15:04:05.000"))
}

// OnBroadcastAck 处理广播 ACK
func (gb *GB28181Plugin) OnBroadcastAck(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnBroadcastAck: invalid from header")
		return
	}

	deviceID := from.Address.User
	ackTime := time.Now()
	gb.Info("OnBroadcastAck: received",
		"deviceId", deviceID,
		"time", ackTime.Format("15:04:05.000"))

	// 查找会话并标记为 ready
	if bs, ok := BroadcastSessions.Find(func(bs *BroadcastSession) bool {
		return bs.Device != nil && bs.Device.DeviceId == deviceID
	}); ok {
		bs.ready = true
		gb.Info("OnBroadcastAck: session ready for audio",
			"deviceId", deviceID,
			"queuedAudioPackets", len(bs.audioChan),
			"time", time.Now().Format("15:04:05.000"))
	}
}

// OnBroadcastBye 处理广播 BYE
func (gb *GB28181Plugin) OnBroadcastBye(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnBroadcastBye: invalid from header")
		return
	}

	deviceID := from.Address.User
	gb.Info("OnBroadcastBye: received", "deviceId", deviceID)

	// 查找并停止广播会话
	if sessionToStop, ok := BroadcastSessions.Find(func(bs *BroadcastSession) bool {
		return bs.Device != nil && bs.Device.DeviceId == deviceID
	}); ok {
		sessionToStop.StopBroadcast()
	}

	// 发送 200 OK
	okResp := sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil)
	if err := tx.Respond(okResp); err != nil {
		gb.Error("OnBroadcastBye: send ok failed", "error", err)
	}
}

// ALawToULawTable A-law 到 μ-law 转换表
var ALawToULawTable = [256]uint8{
	0x2a, 0x2b, 0x28, 0x29, 0x2e, 0x2f, 0x2c, 0x2d,
	0x22, 0x23, 0x20, 0x21, 0x26, 0x27, 0x24, 0x25,
	0x3a, 0x3b, 0x38, 0x39, 0x3e, 0x3f, 0x3c, 0x3d,
	0x32, 0x33, 0x30, 0x31, 0x36, 0x37, 0x34, 0x35,
	0x0a, 0x0b, 0x08, 0x09, 0x0e, 0x0f, 0x0c, 0x0d,
	0x02, 0x03, 0x00, 0x01, 0x06, 0x07, 0x04, 0x05,
	0x1a, 0x1b, 0x18, 0x19, 0x1e, 0x1f, 0x1c, 0x1d,
	0x12, 0x13, 0x10, 0x11, 0x16, 0x17, 0x14, 0x15,
	0x6a, 0x6b, 0x68, 0x69, 0x6e, 0x6f, 0x6c, 0x6d,
	0x62, 0x63, 0x60, 0x61, 0x66, 0x67, 0x64, 0x65,
	0x7a, 0x7b, 0x78, 0x79, 0x7e, 0x7f, 0x7c, 0x7d,
	0x72, 0x73, 0x70, 0x71, 0x76, 0x77, 0x74, 0x75,
	0x4a, 0x4b, 0x48, 0x49, 0x4e, 0x4f, 0x4c, 0x4d,
	0x42, 0x43, 0x40, 0x41, 0x46, 0x47, 0x44, 0x45,
	0x5a, 0x5b, 0x58, 0x59, 0x5e, 0x5f, 0x5c, 0x5d,
	0x52, 0x53, 0x50, 0x51, 0x56, 0x57, 0x54, 0x55,
	0xaa, 0xab, 0xa8, 0xa9, 0xae, 0xaf, 0xac, 0xad,
	0xa2, 0xa3, 0xa0, 0xa1, 0xa6, 0xa7, 0xa4, 0xa5,
	0xba, 0xbb, 0xb8, 0xb9, 0xbe, 0xbf, 0xbc, 0xbd,
	0xb2, 0xb3, 0xb0, 0xb1, 0xb6, 0xb7, 0xb4, 0xb5,
	0x8a, 0x8b, 0x88, 0x89, 0x8e, 0x8f, 0x8c, 0x8d,
	0x82, 0x83, 0x80, 0x81, 0x86, 0x87, 0x84, 0x85,
	0x9a, 0x9b, 0x98, 0x99, 0x9e, 0x9f, 0x9c, 0x9d,
	0x92, 0x93, 0x90, 0x91, 0x96, 0x97, 0x94, 0x95,
	0xea, 0xeb, 0xe8, 0xe9, 0xee, 0xef, 0xec, 0xed,
	0xe2, 0xe3, 0xe0, 0xe1, 0xe6, 0xe7, 0xe4, 0xe5,
	0xfa, 0xfb, 0xf8, 0xf9, 0xfe, 0xff, 0xfc, 0xfd,
	0xf2, 0xf3, 0xf0, 0xf1, 0xf6, 0xf7, 0xf4, 0xf5,
	0xca, 0xcb, 0xc8, 0xc9, 0xce, 0xcf, 0xcc, 0xcd,
	0xc2, 0xc3, 0xc0, 0xc1, 0xc6, 0xc7, 0xc4, 0xc5,
	0xda, 0xdb, 0xd8, 0xd9, 0xde, 0xdf, 0xdc, 0xdd,
	0xd2, 0xd3, 0xd0, 0xd1, 0xd6, 0xd7, 0xd4, 0xd5,
}
