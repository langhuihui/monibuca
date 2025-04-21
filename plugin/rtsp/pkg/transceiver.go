package rtsp

import (
	"fmt"
	"net"
	"reflect"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

type Sender struct {
	*m7s.Subscriber
	Stream
	UDPPorts          map[int][]int          // 保存媒体索引对应的UDP端口 [clientRTP, clientRTCP, serverRTP, serverRTCP]
	UDPConns          map[int][]*net.UDPConn // 保存媒体索引对应的UDP连接 [RTP发送连接, RTCP发送连接]
	AllocatedUDPPorts []uint16               // 记录从端口池分配的UDP端口，用于连接结束时释放回池
}

type Receiver struct {
	*m7s.Publisher
	Stream
	AudioCodecParameters *webrtc.RTPCodecParameters
	VideoCodecParameters *webrtc.RTPCodecParameters
	audioTimestamp       uint32
	lastVideoTimestamp   uint32
	lastAudioPacketTS    uint32    // 上一个音频包的时间戳
	audioTSCheckStart    time.Time // 开始检查音频时间戳的时间
	useVideoTS           bool      // 是否使用视频时间戳
}

func (s *Sender) GetMedia() (medias []*Media, err error) {
	if s.SubAudio && s.Publisher.PubAudio && s.Publisher.HasAudioTrack() {
		audioTrack := s.Publisher.GetAudioTrack(reflect.TypeOf((*mrtp.Audio)(nil)))
		if err = audioTrack.WaitReady(); err != nil {
			return
		}
		parameter := audioTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
		media := &Media{
			Kind:      "audio",
			Direction: DirectionRecvonly,
			Codecs: []*Codec{{
				Name:        parameter.MimeType[6:],
				ClockRate:   parameter.ClockRate,
				Channels:    parameter.Channels,
				FmtpLine:    parameter.SDPFmtpLine,
				PayloadType: uint8(parameter.PayloadType),
			}},
			ID: fmt.Sprintf("trackID=%d", len(medias)),
		}
		s.AudioChannelID = len(medias) << 1
		medias = append(medias, media)
	}

	if s.SubVideo && s.Publisher.PubVideo && s.Publisher.HasVideoTrack() {
		videoTrack := s.Publisher.GetVideoTrack(reflect.TypeOf((*mrtp.Video)(nil)))
		if err = videoTrack.WaitReady(); err != nil {
			return
		}
		parameter := videoTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
		c := Codec{
			Name:        parameter.MimeType[6:],
			ClockRate:   parameter.ClockRate,
			Channels:    parameter.Channels,
			FmtpLine:    parameter.SDPFmtpLine,
			PayloadType: uint8(parameter.PayloadType),
		}
		media := &Media{
			Kind:      "video",
			Direction: DirectionRecvonly,
			Codecs:    []*Codec{&c},
			ID:        fmt.Sprintf("trackID=%d", len(medias)),
		}
		s.VideoChannelID = len(medias) << 1
		medias = append(medias, media)
	}
	return
}

func (s *Sender) sendRTP(pack *mrtp.RTPData, channel int) (err error) {
	mediaIndex := channel / 2 // 计算媒体索引

	// 检查是否使用UDP传输
	if s.Transport == "UDP" && s.UDPPorts != nil {
		if ports, ok := s.UDPPorts[mediaIndex]; ok {
			// 确保UDP连接已经建立
			if s.UDPConns == nil || s.UDPConns[mediaIndex] == nil {
				// 创建UDP连接
				if err = s.setupUDPConnection(mediaIndex); err != nil {
					s.Stream.Error("Failed to setup UDP connection", "error", err, "mediaIndex", mediaIndex)
					goto TCP_FALLBACK // 如果UDP连接失败，回退到TCP
				}
			}

			// 再次检查UDP连接是否已正确建立，以及是否有足够的连接
			if s.UDPConns == nil || len(s.UDPConns[mediaIndex]) < 1 {
				s.Stream.Error("UDP connections not properly established", "mediaIndex", mediaIndex)
				goto TCP_FALLBACK // 如果UDP连接不完整，回退到TCP
			}

			// 获取UDP连接
			rtpConn := s.UDPConns[mediaIndex][0]
			if rtpConn == nil {
				s.Stream.Error("RTP UDP connection is nil", "mediaIndex", mediaIndex)
				goto TCP_FALLBACK
			}

			// 获取客户端IP地址，正确处理IPv6
			remoteIP := s.Conn.RemoteAddr().(*net.TCPAddr).IP
			var clientAddr *net.UDPAddr
			if remoteIP.To4() == nil {
				// IPv6地址需要使用[ip]:port格式
				clientAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("[%s]:%d", remoteIP.String(), ports[0]))
			} else {
				// IPv4地址
				clientAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", remoteIP.String(), ports[0]))
			}

			if err != nil {
				s.Stream.Error("Failed to resolve UDP address", "error", err, "ip", remoteIP.String(), "port", ports[0])
				goto TCP_FALLBACK
			}

			// 发送RTP包
			for _, packet := range pack.Packets {
				buf, err := packet.Marshal()
				if err != nil {
					return err
				}

				_, err = rtpConn.WriteToUDP(buf, clientAddr)
				if err != nil {
					s.Stream.Error("UDP send failed", "error", err, "addr", clientAddr.String())
					goto TCP_FALLBACK
				}
			}

			return nil
		}
	}

TCP_FALLBACK:
	// 使用TCP传输（原有逻辑）
	s.StartWrite()
	defer s.StopWrite()
	for _, packet := range pack.Packets {
		size := packet.MarshalSize()
		chunk := s.MemoryAllocator.Borrow(size + 4)
		chunk[0], chunk[1], chunk[2], chunk[3] = '$', byte(channel), byte(size>>8), byte(size)
		if _, err = packet.MarshalTo(chunk[4:]); err != nil {
			return
		}
		if _, err = s.Write(chunk); err != nil {
			return
		}
	}
	return
}

// 建立UDP连接
func (s *Sender) setupUDPConnection(mediaIndex int) error {
	ports, ok := s.UDPPorts[mediaIndex]
	if !ok {
		return fmt.Errorf("no UDP ports configured for media %d", mediaIndex)
	}

	// 服务器端RTP和RTCP端口
	serverRTPPort := ports[2]
	serverRTCPPort := ports[3]

	s.Stream.Debug("Setting up UDP connections", "mediaIndex", mediaIndex,
		"serverRTPPort", serverRTPPort, "serverRTCPPort", serverRTCPPort)

	// 尝试创建RTP连接，支持重试几次
	var rtpConn *net.UDPConn
	var rtcpConn *net.UDPConn
	var err error
	maxRetries := 3

	// 创建RTP连接
	for i := 0; i < maxRetries; i++ {
		rtpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", serverRTPPort+i*2))
		if err != nil {
			s.Stream.Error("Failed to resolve RTP UDP address", "error", err, "port", serverRTPPort+i*2)
			continue
		}

		rtpConn, err = net.ListenUDP("udp", rtpAddr)
		if err != nil {
			s.Stream.Error("Failed to listen on RTP UDP port", "error", err, "port", serverRTPPort+i*2)
			continue
		}

		// 成功创建RTP连接
		serverRTPPort = serverRTPPort + i*2
		serverRTCPPort = serverRTPPort + 1 // 更新RTCP端口为RTP+1
		break
	}

	if rtpConn == nil {
		return fmt.Errorf("failed to create RTP UDP connection after %d retries", maxRetries)
	}

	// 创建RTCP连接
	rtcpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", serverRTCPPort))
	if err != nil {
		rtpConn.Close()
		return fmt.Errorf("failed to resolve RTCP UDP address: %w", err)
	}

	rtcpConn, err = net.ListenUDP("udp", rtcpAddr)
	if err != nil {
		rtpConn.Close()
		return fmt.Errorf("failed to listen on RTCP UDP port %d: %w", serverRTCPPort, err)
	}

	// 初始化UDP连接映射
	if s.UDPConns == nil {
		s.UDPConns = make(map[int][]*net.UDPConn)
	}

	// 保存连接
	s.UDPConns[mediaIndex] = []*net.UDPConn{rtpConn, rtcpConn}

	// 更新端口信息（如果有变化）
	if ports[2] != serverRTPPort || ports[3] != serverRTCPPort {
		s.UDPPorts[mediaIndex][2] = serverRTPPort
		s.UDPPorts[mediaIndex][3] = serverRTCPPort
	}

	s.Stream.Info("UDP connections established", "mediaIndex", mediaIndex,
		"rtpPort", serverRTPPort, "rtcpPort", serverRTCPPort)

	return nil
}

// 发送RTCP包
func (s *Sender) sendRTCP(packet rtcp.Packet, mediaIndex int) error {
	// 检查是否使用UDP传输
	if s.Transport == "UDP" && s.UDPPorts != nil && s.UDPConns != nil {
		// 检查mediaIndex是否存在于UDPPorts和UDPConns中
		ports, portsOk := s.UDPPorts[mediaIndex]
		conns, connsOk := s.UDPConns[mediaIndex]

		if portsOk && connsOk && len(conns) > 1 && conns[1] != nil {
			rtcpConn := conns[1]

			// 获取客户端IP地址，正确处理IPv6
			remoteIP := s.Conn.RemoteAddr().(*net.TCPAddr).IP
			var clientAddr *net.UDPAddr
			var err error

			if remoteIP.To4() == nil {
				// IPv6地址需要使用[ip]:port格式
				clientAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("[%s]:%d", remoteIP.String(), ports[1]))
			} else {
				// IPv4地址
				clientAddr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", remoteIP.String(), ports[1]))
			}

			if err != nil {
				s.Stream.Error("Failed to resolve RTCP UDP address", "error", err, "ip", remoteIP.String(), "port", ports[1])
				return err
			}

			// 序列化并发送RTCP包
			buf, err := packet.Marshal()
			if err != nil {
				return err
			}

			_, err = rtcpConn.WriteToUDP(buf, clientAddr)
			if err != nil {
				s.Stream.Error("UDP RTCP send failed", "error", err, "addr", clientAddr.String())
				return err
			}

			return nil
		} else {
			connsLen := 0
			if connsOk {
				connsLen = len(conns)
			}

			s.Stream.Debug("RTCP UDP connection not available", "mediaIndex", mediaIndex,
				"portsOk", portsOk, "connsOk", connsOk, "connsLen", connsLen)
		}
	}

	// TCP模式发送RTCP或UDP不可用（这里可以根据需要实现）
	return nil
}

func (s *Sender) Send() (err error) {
	// 启动音视频发送协程
	go m7s.PlayBlock(s.Subscriber, func(audio *mrtp.Audio) error {
		return s.sendRTP(&audio.RTPData, s.AudioChannelID)
	}, func(video *mrtp.Video) error {
		return s.sendRTP(&video.RTPData, s.VideoChannelID)
	})

	// 如果是UDP模式，需要在这里启动一个RTCP处理协程
	if s.Transport == "UDP" {
		done := s.Subscriber.Done()
		go func() {
			rtcpTicker := time.NewTicker(5 * time.Second)
			defer rtcpTicker.Stop()

			// 创建RTCP SR包
			sr := &rtcp.SenderReport{
				SSRC:        0, // 将在实际发送时设置
				NTPTime:     0,
				RTPTime:     0,
				PacketCount: 0,
				OctetCount:  0,
			}

			// 创建SDES包
			sdes := &rtcp.SourceDescription{
				Chunks: []rtcp.SourceDescriptionChunk{
					{
						Source: 0, // 将在实际发送时设置
						Items: []rtcp.SourceDescriptionItem{
							{Type: rtcp.SDESCNAME, Text: "monibuca"},
						},
					},
				},
			}

			for {
				select {
				case <-rtcpTicker.C:
					// 更新NTP时间 (RFC3550中定义的格式)
					now := time.Now()
					seconds := uint32(now.Unix() + 0x83AA7E80) // 从1900年开始的秒数，加上从1900到1970的秒数
					fraction := uint32(now.Nanosecond() * 0x100000000 / 1000000000)
					ntp := uint64(seconds)<<32 | uint64(fraction)
					sr.NTPTime = ntp

					// 向所有媒体轨道发送RTCP
					if s.AudioChannelID >= 0 {
						mediaIndex := s.AudioChannelID / 2
						s.sendRTCP(sr, mediaIndex)
						s.sendRTCP(sdes, mediaIndex)
					}

					if s.VideoChannelID >= 0 {
						mediaIndex := s.VideoChannelID / 2
						s.sendRTCP(sr, mediaIndex)
						s.sendRTCP(sdes, mediaIndex)
					}

				case <-done:
					return
				}
			}
		}()
	}

	// 接收处理（处理客户端发来的消息）
	return s.NetConnection.Receive(true, nil, nil)
}

func (r *Receiver) SetMedia(medias []*Media) (err error) {
	r.AudioChannelID = -1
	r.VideoChannelID = -1
	for i, media := range medias {
		if codec := media.Codecs[0]; codec.IsAudio() {
			r.AudioCodecParameters = &webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "audio/" + codec.Name,
					ClockRate:    codec.ClockRate,
					Channels:     codec.Channels,
					SDPFmtpLine:  codec.FmtpLine,
					RTCPFeedback: nil,
				},
				PayloadType: webrtc.PayloadType(codec.PayloadType),
			}
			r.AudioChannelID = i << 1
		} else if codec.IsVideo() {
			r.VideoChannelID = i << 1
			r.VideoCodecParameters = &webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/" + codec.Name,
					ClockRate:    codec.ClockRate,
					Channels:     codec.Channels,
					SDPFmtpLine:  codec.FmtpLine,
					RTCPFeedback: nil,
				},
				PayloadType: webrtc.PayloadType(codec.PayloadType),
			}
		} else {
			r.Stream.Warn("media kind not support", "kind", codec.Kind())
		}
	}
	return
}

func (r *Receiver) Receive() (err error) {
	audioFrame, videoFrame := &mrtp.Audio{}, &mrtp.Video{}
	audioFrame.SetAllocator(r.MemoryAllocator)
	audioFrame.RTPCodecParameters = r.AudioCodecParameters
	videoFrame.SetAllocator(r.MemoryAllocator)
	videoFrame.RTPCodecParameters = r.VideoCodecParameters
	var rtcpTS time.Time
	sdes := &rtcp.SourceDescription{
		Chunks: []rtcp.SourceDescriptionChunk{
			{
				Source: 0, // Set appropriate SSRC
				Items: []rtcp.SourceDescriptionItem{
					{Type: rtcp.SDESCNAME, Text: "monibuca"}, // Set appropriate CNAME
				},
			},
		},
	}
	rr := &rtcp.ReceiverReport{
		Reports: []rtcp.ReceptionReport{
			{
				SSRC:               0, // Set appropriate SSRC
				FractionLost:       0, // Set appropriate fraction lost
				TotalLost:          0, // Set total packets lost
				LastSequenceNumber: 0, // Set last sequence number
				Jitter:             0, // Set jitter
				LastSenderReport:   0, // Set last SR timestamp
				Delay:              0, // Set delay since last SR
			},
		},
	}
	return r.NetConnection.Receive(false, func(channelID byte, buf []byte) error {
		if r.Publisher.Paused != nil {
			r.Stream.Pause()
			r.Publisher.Paused.Await()
			r.Stream.Play()
		}
		if time.Since(rtcpTS) > 5*time.Second {
			rtcpTS = time.Now()
			// Serialize RTCP packets
			rawRR, err := rr.Marshal()
			if err != nil {
				return err
			}
			rawSDES, err := sdes.Marshal()
			if err != nil {
				return err
			}
			length := len(rawRR) + len(rawSDES)
			rtcp := append([]byte{'$', 0x03, byte(length >> 8), byte(length)}, rawRR...)
			rtcp = append(rtcp, rawSDES...)
			// Send RTCP packets
			if _, err = r.NetConnection.Write(rtcp); err != nil {
				return err
			}
		}
		switch int(channelID) {
		case r.AudioChannelID:
			if !r.PubAudio {
				return pkg.ErrMuted
			}
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return err
			}
			rr.SSRC = packet.SSRC
			sdes.Chunks[0].Source = packet.SSRC
			rr.Reports[0].SSRC = packet.SSRC
			rr.Reports[0].LastSequenceNumber = uint32(packet.SequenceNumber)

			now := time.Now()
			// 检查音频时间戳是否变化
			if r.lastAudioPacketTS == 0 {
				r.lastAudioPacketTS = packet.Timestamp
				r.audioTSCheckStart = now
				r.Stream.Debug("check audio timestamp start", "firsttime", "timestamp", packet.Timestamp)
			} else if !r.useVideoTS {
				r.Stream.Debug("debug audio timestamp", "current", packet.Timestamp, "last", r.lastAudioPacketTS, "duration", now.Sub(r.audioTSCheckStart))
				// 如果3秒内时间戳没有变化，切换到使用视频时间戳
				if packet.Timestamp == r.lastAudioPacketTS && now.Sub(r.audioTSCheckStart) > 3*time.Second {
					r.useVideoTS = true
					r.Stream.Debug("switch to video timestamp due to unchanging audio timestamp")
					packet.Timestamp = uint32(float64(r.lastVideoTimestamp) * 8000 / 90000)
					audioFrame = &mrtp.Audio{}
					audioFrame.AddRecycleBytes(buf)
					audioFrame.Packets = []*rtp.Packet{packet}
					audioFrame.RTPCodecParameters = r.AudioCodecParameters
					audioFrame.SetAllocator(r.MemoryAllocator)
					return pkg.ErrDiscard
				} else if packet.Timestamp != r.lastAudioPacketTS {
					// 时间戳有变化，重置检查
					r.lastAudioPacketTS = packet.Timestamp
					r.audioTSCheckStart = now
					r.Stream.Debug("check audio timestamp start", "reset audioTSCheckStart", "lastAudioPacketTS", r.lastAudioPacketTS)
				}
			}

			// 如果检测到时间戳异常，使用视频时间戳
			if r.useVideoTS {
				packet.Timestamp = uint32(float64(r.lastVideoTimestamp) * 8000 / 90000)
			}

			if len(audioFrame.Packets) == 0 || packet.Timestamp == audioFrame.Packets[0].Timestamp {
				audioFrame.AddRecycleBytes(buf)
				audioFrame.Packets = append(audioFrame.Packets, packet)
				return pkg.ErrDiscard
			} else {
				if err = r.WriteAudio(audioFrame); err != nil {
					return err
				}
				audioFrame = &mrtp.Audio{}
				audioFrame.AddRecycleBytes(buf)
				audioFrame.Packets = []*rtp.Packet{packet}
				audioFrame.RTPCodecParameters = r.AudioCodecParameters
				audioFrame.SetAllocator(r.MemoryAllocator)
				return pkg.ErrDiscard
			}
		case r.VideoChannelID:
			if !r.PubVideo {
				return pkg.ErrMuted
			}
			packet := &rtp.Packet{}
			if err = packet.Unmarshal(buf); err != nil {
				return err
			}
			rr.Reports[0].SSRC = packet.SSRC
			sdes.Chunks[0].Source = packet.SSRC
			rr.Reports[0].LastSequenceNumber = uint32(packet.SequenceNumber)
			r.lastVideoTimestamp = packet.Timestamp
			if len(videoFrame.Packets) == 0 || packet.Timestamp == videoFrame.Packets[0].Timestamp {
				videoFrame.AddRecycleBytes(buf)
				videoFrame.Packets = append(videoFrame.Packets, packet)
				return pkg.ErrDiscard
			} else {
				// t := time.Now()
				if err = r.WriteVideo(videoFrame); err != nil {
					return err
				}
				// fmt.Println("write video", time.Since(t))
				videoFrame = &mrtp.Video{}
				videoFrame.AddRecycleBytes(buf)
				videoFrame.Packets = []*rtp.Packet{packet}
				videoFrame.RTPCodecParameters = r.VideoCodecParameters
				videoFrame.SetAllocator(r.MemoryAllocator)
				return pkg.ErrDiscard
			}
		default:

		}
		return pkg.ErrUnsupportCodec
	}, func(channelID byte, buf []byte) error {
		msg := &RTCP{Channel: channelID}
		if err = msg.Header.Unmarshal(buf); err != nil {
			return err
		}
		if msg.Packets, err = rtcp.Unmarshal(buf); err != nil {
			return err
		}
		r.Stream.Debug("rtcp", "type", msg.Header.Type, "length", msg.Header.Length)
		if msg.Header.Type == rtcp.TypeSenderReport {
			for _, report := range msg.Packets {
				if report, ok := report.(*rtcp.SenderReport); ok {
					rr.Reports[0].LastSenderReport = uint32(report.NTPTime)
				}
			}

		}
		return pkg.ErrDiscard
	})
}

// 添加Dispose方法，清理资源
func (r *Receiver) Dispose() {
	// 清理可能持有的帧资源
	if r.Publisher != nil {
		// 如果必要，这里可以添加额外的Publisher清理代码
	}

	// 调用基类Dispose
	r.Stream.Dispose()
	r.Stream.Info("Receiver disposed and resources cleaned up")
}

// 添加Dispose方法，清理UDP资源
func (s *Sender) Dispose() {
	// 释放UDP连接资源
	if s.UDPConns != nil {
		for mediaIndex, conns := range s.UDPConns {
			for i, conn := range conns {
				if conn != nil {
					connType := "RTP"
					if i == 1 {
						connType = "RTCP"
					}

					err := conn.Close()
					if err != nil {
						s.Stream.Error("Error closing UDP connection", "error", err,
							"mediaIndex", mediaIndex, "type", connType)
					} else {
						s.Stream.Debug("Closed UDP connection", "mediaIndex", mediaIndex, "type", connType)
					}
				}
			}
		}
		s.UDPConns = nil
	}

	// 调用基类Dispose
	s.Stream.Dispose()
}
