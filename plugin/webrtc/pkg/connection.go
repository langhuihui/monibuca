package webrtc

import (
	"errors"
	"fmt"
	"net"     // Add this import
	"strings" // Add this import
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	. "github.com/pion/webrtc/v4"
	"m7s.live/v5"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
)

type Connection struct {
	*PeerConnection
	SupportsH265 bool // Add this field
	Publisher    *m7s.Publisher
	SDP          string
}

func (IO *Connection) GetOffer() (*SessionDescription, error) {
	offer, err := IO.CreateOffer(nil)
	if err != nil {
		return nil, err
	}
	gatherComplete := GatheringCompletePromise(IO.PeerConnection)
	if err = IO.SetLocalDescription(offer); err != nil {
		return nil, err
	}
	<-gatherComplete
	return IO.LocalDescription(), nil
}

func (IO *Connection) GetAnswer() (*SessionDescription, error) {
	answer, err := IO.CreateAnswer(nil)
	if err != nil {
		return nil, err
	}
	gatherComplete := GatheringCompletePromise(IO.PeerConnection)
	if err = IO.SetLocalDescription(answer); err != nil {
		return nil, err
	}
	<-gatherComplete
	return IO.LocalDescription(), nil
}

type MultipleConnection struct {
	task.Task
	Connection
	// LocalSDP *sdp.SessionDescription
	Subscriber *m7s.Subscriber
	EnableDC   bool
	PLI        time.Duration
}

func (IO *MultipleConnection) Start() (err error) {
	if IO.Publisher != nil {
		IO.Depend(IO.Publisher)
		IO.Publisher.Depend(IO)
		IO.Receive()
	}
	if IO.Subscriber != nil {
		IO.Depend(IO.Subscriber)
		IO.Subscriber.Depend(IO)
		IO.Send()
	}
	IO.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			IO.Info(ice.ToJSON().Candidate)
		}
	})
	// 监听ICE连接状态变化
	IO.OnICEConnectionStateChange(func(state ICEConnectionState) {
		IO.Debug("ICE connection state changed", "state", state.String())
		if state == ICEConnectionStateFailed {
			IO.Error("ICE connection failed")
		}
	})

	IO.OnConnectionStateChange(func(state PeerConnectionState) {
		IO.Info("Connection State has changed:" + state.String())
		switch state {
		case PeerConnectionStateConnected:

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed, PeerConnectionStateClosed:
			IO.Stop(errors.New("connection state:" + state.String()))
		}
	})
	return
}

func (IO *MultipleConnection) Receive() {
	puber := IO.Publisher
	IO.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
		IO.Info("OnTrack", "kind", track.Kind().String(), "payloadType", uint8(track.Codec().PayloadType))
		var n int
		var err error
		if codecP := track.Codec(); track.Kind() == RTPCodecTypeAudio {
			if !puber.PubAudio {
				return
			}
			mem := util.NewScalableMemoryAllocator(1 << 12)
			defer mem.Recycle()
			frame := &mrtp.Audio{}
			frame.RTPCodecParameters = &codecP
			frame.SetAllocator(mem)
			for {
				var packet rtp.Packet
				buf := mem.Malloc(mrtp.MTUSize)
				if n, _, err = track.Read(buf); err == nil {
					mem.FreeRest(&buf, n)
					err = packet.Unmarshal(buf)
				}
				if err != nil {
					return
				}
				if len(packet.Payload) == 0 {
					mem.Free(buf)
					continue
				}
				if len(frame.Packets) == 0 || packet.Timestamp == frame.Packets[0].Timestamp {
					frame.AddRecycleBytes(buf)
					frame.Packets = append(frame.Packets, &packet)
				} else {
					err = puber.WriteAudio(frame)
					frame = &mrtp.Audio{}
					frame.AddRecycleBytes(buf)
					frame.Packets = []*rtp.Packet{&packet}
					frame.RTPCodecParameters = &codecP
					frame.SetAllocator(mem)
				}
			}
		} else {
			if !puber.PubVideo {
				return
			}
			var lastPLISent time.Time
			mem := util.NewScalableMemoryAllocator(1 << 12)
			defer mem.Recycle()
			frame := &mrtp.Video{}
			frame.RTPCodecParameters = &codecP
			frame.SetAllocator(mem)
			for {
				if time.Since(lastPLISent) > IO.PLI {
					if rtcpErr := IO.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
						puber.Error("writeRTCP", "error", rtcpErr)
						return
					}
					lastPLISent = time.Now()
				}
				var packet rtp.Packet
				buf := mem.Malloc(mrtp.MTUSize)
				if n, _, err = track.Read(buf); err == nil {
					mem.FreeRest(&buf, n)
					err = packet.Unmarshal(buf)
				}
				if err != nil {
					return
				}
				if len(packet.Payload) == 0 {
					mem.Free(buf)
					continue
				}
				if len(frame.Packets) == 0 || packet.Timestamp == frame.Packets[0].Timestamp {
					frame.AddRecycleBytes(buf)
					frame.Packets = append(frame.Packets, &packet)
				} else {
					err = puber.WriteVideo(frame)
					frame = &mrtp.Video{}
					frame.AddRecycleBytes(buf)
					frame.Packets = []*rtp.Packet{&packet}
					frame.RTPCodecParameters = &codecP
					frame.SetAllocator(mem)
				}
			}
		}
	})
	IO.OnDataChannel(func(d *DataChannel) {
		IO.Info("OnDataChannel", "label", d.Label())
		d.OnMessage(func(msg DataChannelMessage) {
			IO.SDP = string(msg.Data[1:])
			IO.Debug("dc message", "sdp", IO.SDP)
			if err := IO.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: IO.SDP}); err != nil {
				return
			}
			if answer, err := IO.GetAnswer(); err == nil {
				d.SendText(answer.SDP)
			} else {
				return
			}
			switch msg.Data[0] {
			case '0':
				IO.Stop(errors.New("stop by remote"))
			case '1':

			}
		})
	})
}

func (IO *MultipleConnection) SendSubscriber(subscriber *m7s.Subscriber) (audioSender, videoSender *RTPSender, err error) {
	var useDC bool
	var audioTLSRTP, videoTLSRTP *TrackLocalStaticRTP
	vctx, actx := subscriber.Publisher.GetVideoCodecCtx(), subscriber.Publisher.GetAudioCodecCtx()
	if IO.EnableDC {
		// If H265 is supported by the client, we do NOT use DataChannel for H265 video.
		// DataChannel will be used for H265 video only if the client does NOT support H265 (potentially for transcoding or specific handling).
		// Or if video is not H265 but DC is enabled for other codecs like MP4A audio.
		if vctx != nil && vctx.FourCC() == codec.FourCC_H265 {
			if !IO.SupportsH265 { // Client does not support H265, so use DC
				useDC = true
				IO.Info("Client does not support H265, using DataChannel for H265 video.")
			} else {
				// Client supports H265, so we will use RTP. useDC remains false.
				IO.Info("Client supports H265, using RTP for H265 video.")
			}
		} else if actx != nil && actx.FourCC() == codec.FourCC_MP4A { // For MP4A audio, use DC if enabled
			useDC = true
		}
	}
	if vctx != nil && !useDC {
		videoCodec := vctx.FourCC()
		var rcc RTPCodecParameters
		if ctx, ok := vctx.(mrtp.IRTPCtx); ok {
			rcc = ctx.GetRTPCodecParameter()
		} else {
			var rtpCtx mrtp.RTPData
			var tmpAVTrack AVTrack
			tmpAVTrack.ICodecCtx, _, err = rtpCtx.ConvertCtx(vctx)
			if err == nil {
				rcc = tmpAVTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
			} else {
				return
			}
		}
		rcc.RTCPFeedback = videoRTCPFeedback
		videoTLSRTP, err = NewTrackLocalStaticRTP(rcc.RTPCodecCapability, videoCodec.String(), subscriber.StreamPath)
		if err != nil {
			return
		}
		videoSender, err = IO.PeerConnection.AddTrack(videoTLSRTP)
		if err != nil {
			return
		}
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				if n, _, rtcpErr := videoSender.Read(rtcpBuf); rtcpErr != nil {
					subscriber.Warn("rtcp read error", "error", rtcpErr)
					return
				} else {
					if p, err := rtcp.Unmarshal(rtcpBuf[:n]); err == nil {
						for _, pp := range p {
							switch pp.(type) {
							case *rtcp.PictureLossIndication:
								// fmt.Println("PictureLossIndication")
							}
						}
					}
				}
			}
		}()
	}
	if actx != nil && !useDC && actx.FourCC() != codec.FourCC_MP4A {
		audioCodec := actx.FourCC()
		var rcc RTPCodecParameters
		if ctx, ok := actx.(mrtp.IRTPCtx); ok {
			rcc = ctx.GetRTPCodecParameter()
		} else {
			var rtpCtx mrtp.RTPData
			var tmpAVTrack AVTrack
			tmpAVTrack.ICodecCtx, _, err = rtpCtx.ConvertCtx(actx)
			if err == nil {
				rcc = tmpAVTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
			} else {
				return
			}
		}

		// Transform SDPFmtpLine for WebRTC compatibility (primarily for video codecs, but general logic)
		mimeTypeLower := strings.ToLower(rcc.RTPCodecCapability.MimeType)
		if strings.Contains(mimeTypeLower, "h264") || strings.Contains(mimeTypeLower, "h265") { // This condition will likely not match for typical audio codecs
			originalFmtpLine := rcc.RTPCodecCapability.SDPFmtpLine
			parts := strings.Split(originalFmtpLine, ";")
			var newParts []string
			for _, part := range parts {
				trimmedPart := strings.TrimSpace(part)
				if !strings.HasPrefix(trimmedPart, "sprop-parameter-sets=") {
					newParts = append(newParts, trimmedPart)
				}
			}
			transformedFmtpLine := strings.Join(newParts, ";")
			if transformedFmtpLine != originalFmtpLine {
				rcc.RTPCodecCapability.SDPFmtpLine = transformedFmtpLine
				IO.Info("Adjusted SDPFmtpLine for WebRTC (audio track context)", "codec", rcc.RTPCodecCapability.MimeType, "from", originalFmtpLine, "to", transformedFmtpLine)
			}
		}

		audioTLSRTP, err = NewTrackLocalStaticRTP(rcc.RTPCodecCapability, audioCodec.String(), subscriber.StreamPath)
		if err != nil {
			return
		}
		audioSender, err = IO.PeerConnection.AddTrack(audioTLSRTP)
		if err != nil {
			return
		}
	}
	var dc *DataChannel
	if useDC {
		dc, err = IO.CreateDataChannel(subscriber.StreamPath, nil)
		if err != nil {
			return
		}
		dc.OnOpen(func() {
			var live flv.Live
			live.WriteFlvTag = func(buffers net.Buffers) (err error) {
				r := util.NewReadableBuffersFromBytes(buffers...)
				for r.Length > 65535 {
					r.RangeN(65535, func(buf []byte) {
						err = dc.Send(buf)
						if err != nil {
							fmt.Println("dc send error", err)
						}
					})
				}
				r.Range(func(buf []byte) {
					err = dc.Send(buf)
					if err != nil {
						fmt.Println("dc send error", err)
					}
				})
				return
			}
			live.Subscriber = subscriber
			err = live.Run()
			dc.Close()
		})
	} else {
		if audioSender == nil {
			subscriber.SubAudio = false
		}
		if videoSender == nil {
			subscriber.SubVideo = false
		}
		go m7s.PlayBlock(subscriber, func(frame *mrtp.Audio) (err error) {
			for _, p := range frame.Packets {
				if err = audioTLSRTP.WriteRTP(p); err != nil {
					return
				}
			}
			return
		}, func(frame *mrtp.Video) error {
			for _, p := range frame.Packets {
				if err := videoTLSRTP.WriteRTP(p); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return
}

func (IO *MultipleConnection) Send() (err error) {
	if IO.Subscriber != nil {
		_, _, err = IO.SendSubscriber(IO.Subscriber)
	}
	return
}

func (IO *MultipleConnection) Dispose() {
	IO.PeerConnection.Close()
}

type RemoteStream struct {
	task.Task
	pc          *Connection
	suber       *m7s.Subscriber
	videoTLSRTP *TrackLocalStaticRTP
	videoSender *RTPSender
}

func (r *RemoteStream) GetKey() string {
	return r.suber.StreamPath
}

func (r *RemoteStream) Start() (err error) {
	vctx := r.suber.Publisher.GetVideoCodecCtx()
	videoCodec := vctx.FourCC()
	var rcc RTPCodecParameters
	if ctx, ok := vctx.(mrtp.IRTPCtx); ok {
		rcc = ctx.GetRTPCodecParameter()
	} else {
		var rtpCtx mrtp.RTPData
		var tmpAVTrack AVTrack
		tmpAVTrack.ICodecCtx, _, err = rtpCtx.ConvertCtx(vctx)
		if err == nil {
			rcc = tmpAVTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
		} else {
			return
		}
	}

	r.videoTLSRTP, err = NewTrackLocalStaticRTP(rcc.RTPCodecCapability, videoCodec.String(), r.suber.StreamPath)
	if err != nil {
		return
	}
	r.videoSender, err = r.pc.AddTrack(r.videoTLSRTP)
	return
}

func (r *RemoteStream) Go() (err error) {
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if n, _, rtcpErr := r.videoSender.Read(rtcpBuf); rtcpErr != nil {
				r.suber.Warn("rtcp read error", "error", rtcpErr)
				return
			} else {
				if p, err := rtcp.Unmarshal(rtcpBuf[:n]); err == nil {
					for _, pp := range p {
						switch pp.(type) {
						case *rtcp.PictureLossIndication:
							// fmt.Println("PictureLossIndication")
						}
					}
				}
			}
		}
	}()
	return m7s.PlayBlock(r.suber, (func(frame *mrtp.Audio) (err error))(nil), func(frame *mrtp.Video) error {
		for _, p := range frame.Packets {
			if err := r.videoTLSRTP.WriteRTP(p); err != nil {
				return err
			}
		}
		return nil
	})
}

// SingleConnection extends Connection to handle multiple subscribers in a single WebRTC connection
type SingleConnection struct {
	task.Manager[string, *RemoteStream]
	Connection
}

func (c *SingleConnection) Receive() {
	c.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
		c.Info("OnTrack", "kind", track.Kind().String(), "payloadType", uint8(track.Codec().PayloadType))
	})
}

// AddSubscriber adds a new subscriber to the connection and starts sending
func (c *SingleConnection) AddSubscriber(subscriber *m7s.Subscriber) (remoteStream *RemoteStream) {
	remoteStream = &RemoteStream{suber: subscriber, pc: &c.Connection}
	subscriber.Depend(remoteStream)
	c.Add(remoteStream)
	return
}

// RemoveSubscriber removes a subscriber from the connection
func (c *SingleConnection) RemoveSubscriber(remoteStream *RemoteStream) {
	c.RemoveTrack(remoteStream.videoSender)
	remoteStream.Stop(task.ErrStopByUser)
}

// HasSubscriber checks if a stream is already subscribed
func (c *SingleConnection) HasSubscriber(streamPath string) bool {
	return c.Has(streamPath)
}
