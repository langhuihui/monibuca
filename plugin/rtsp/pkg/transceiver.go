package rtsp

import (
	"fmt"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"m7s.live/m7s/v5"
	mrtp "m7s.live/m7s/v5/plugin/rtp/pkg"
	"reflect"
)

type Stream struct {
	*NetConnection
	AudioChannelID int
	VideoChannelID int
}
type Sender struct {
	*m7s.Subscriber
	Stream
}

type Receiver struct {
	*m7s.Publisher
	Stream
	AudioCodecParameters *webrtc.RTPCodecParameters
	VideoCodecParameters *webrtc.RTPCodecParameters
}

func (ns *Stream) Close() error {
	if ns.NetConnection != nil {
		ns.NetConnection.Destroy()
	}
	return nil
}

func (s *Sender) GetMedia() (medias []*Media, err error) {
	if s.SubAudio && s.Publisher.PubAudio && s.Publisher.HasAudioTrack() {
		audioTrack := s.Publisher.GetAudioTrack(reflect.TypeOf((*mrtp.RTPAudio)(nil)))
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
		videoTrack := s.Publisher.GetVideoTrack(reflect.TypeOf((*mrtp.RTPVideo)(nil)))
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

func (s *Sender) send() error {
	return m7s.PlayBlock(s.Subscriber, func(audio *mrtp.RTPAudio) error {
		return s.sendRTP(&audio.RTPData, s.AudioChannelID)
	}, func(video *mrtp.RTPVideo) error {
		return s.sendRTP(&video.RTPData, s.VideoChannelID)
	})
}

func (s *Sender) receive() {
	var err error
	for err == nil {
		_, _, err = s.NetConnection.Receive(true)
	}
}

func (s *Sender) Send() (err error) {
	go s.receive()
	return s.send()
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
			r.Warn("media kind not support", "kind", codec.Kind())
		}
	}
	return
}
func (r *Receiver) Receive() (err error) {
	audioFrame, videoFrame := &mrtp.RTPAudio{}, &mrtp.RTPVideo{}
	audioFrame.SetAllocator(r.MemoryAllocator)
	audioFrame.RTPCodecParameters = r.AudioCodecParameters
	videoFrame.SetAllocator(r.MemoryAllocator)
	videoFrame.RTPCodecParameters = r.VideoCodecParameters
	var channelID byte
	var buf []byte
	for err == nil {
		channelID, buf, err = r.NetConnection.Receive(false)
		if err != nil {
			return
		}
		if len(buf) == 0 {
			continue
		}
		if channelID&1 == 0 {
			switch int(channelID) {
			case r.AudioChannelID:
				if !r.PubAudio {
					continue
				}
				packet := &rtp.Packet{}
				if err = packet.Unmarshal(buf); err != nil {
					return
				}
				if len(audioFrame.Packets) == 0 || packet.Timestamp == audioFrame.Packets[0].Timestamp {
					audioFrame.AddRecycleBytes(buf)
					audioFrame.Packets = append(audioFrame.Packets, packet)
				} else {
					err = r.WriteAudio(audioFrame)
					audioFrame = &mrtp.RTPAudio{}
					audioFrame.AddRecycleBytes(buf)
					audioFrame.Packets = []*rtp.Packet{packet}
					audioFrame.RTPCodecParameters = r.VideoCodecParameters
					audioFrame.SetAllocator(r.MemoryAllocator)
				}
			case r.VideoChannelID:
				if !r.PubVideo {
					continue
				}
				packet := &rtp.Packet{}
				if err = packet.Unmarshal(buf); err != nil {
					return
				}
				if len(videoFrame.Packets) == 0 || packet.Timestamp == videoFrame.Packets[0].Timestamp {
					videoFrame.AddRecycleBytes(buf)
					videoFrame.Packets = append(videoFrame.Packets, packet)
				} else {
					// t := time.Now()
					err = r.WriteVideo(videoFrame)
					// fmt.Println("write video", time.Since(t))
					videoFrame = &mrtp.RTPVideo{}
					videoFrame.AddRecycleBytes(buf)
					videoFrame.Packets = []*rtp.Packet{packet}
					videoFrame.RTPCodecParameters = r.VideoCodecParameters
					videoFrame.SetAllocator(r.MemoryAllocator)
				}
			default:

			}
		} else {
			msg := &RTCP{Channel: channelID}
			r.MemoryAllocator.Free(buf)
			if err = msg.Header.Unmarshal(buf); err != nil {
				return
			}
			if msg.Packets, err = rtcp.Unmarshal(buf); err != nil {
				return
			}
			r.Debug("rtcp", "type", msg.Header.Type, "length", msg.Header.Length)
			// TODO: rtcp msg
		}
	}
	return
}
