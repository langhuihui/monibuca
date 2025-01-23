package rtsp

import (
	"fmt"
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

func (s *Sender) Send() (err error) {
	go m7s.PlayBlock(s.Subscriber, func(audio *mrtp.Audio) error {
		return s.sendRTP(&audio.RTPData, s.AudioChannelID)
	}, func(video *mrtp.Video) error {
		return s.sendRTP(&video.RTPData, s.VideoChannelID)
	})
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
					return nil
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
				return nil
			} else {
				if err = r.WriteAudio(audioFrame); err != nil {
					return err
				}
				audioFrame = &mrtp.Audio{}
				audioFrame.AddRecycleBytes(buf)
				audioFrame.Packets = []*rtp.Packet{packet}
				audioFrame.RTPCodecParameters = r.AudioCodecParameters
				audioFrame.SetAllocator(r.MemoryAllocator)
				return nil
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
				return nil
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
				return nil
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
