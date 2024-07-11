package plugin_webrtc

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	. "github.com/pion/webrtc/v4"
	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	mrtp "m7s.live/m7s/v5/plugin/rtp/pkg"
	. "m7s.live/m7s/v5/plugin/webrtc/pkg"
)

var (
	//go:embed publish.html
	publishHTML []byte

	//go:embed subscribe.html
	subscribeHTML []byte
	reg_level     = regexp.MustCompile("profile-level-id=(4.+f)")
	_             = m7s.InstallPlugin[WebRTCPlugin]()
)

type WebRTCPlugin struct {
	m7s.Plugin
	ICEServers []ICEServer   `desc:"ice服务器配置"`
	PublicIP   string        `desc:"公网IP"`
	PublicIPv6 string        `desc:"公网IPv6"`
	Port       string        `default:"tcp:9000" desc:"监听端口"`
	PLI        time.Duration `default:"2s" desc:"发送PLI请求间隔"`          // 视频流丢包后，发送PLI请求
	EnableOpus bool          `default:"true" desc:"是否启用opus编码"`       // 是否启用opus编码
	EnableVP9  bool          `default:"false" desc:"是否启用vp9编码"`       // 是否启用vp9编码
	EnableAv1  bool          `default:"false" desc:"是否启用av1编码"`       // 是否启用av1编码
	EnableDC   bool          `default:"false" desc:"是否启用DataChannel"` // 在不支持编码格式的情况下是否启用DataChannel传输
	m          MediaEngine
	s          SettingEngine
	api        *API
}

func (p *WebRTCPlugin) OnInit() (err error) {
	if len(p.ICEServers) > 0 {
		for i := range p.ICEServers {
			b, _ := p.ICEServers[i].MarshalJSON()
			p.ICEServers[i].UnmarshalJSON(b)
		}
	}
	RegisterCodecs(&p.m)
	if p.EnableOpus {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
			PayloadType:        111,
		}, RTPCodecTypeAudio)
	}
	if p.EnableVP9 {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeVP9, 90000, 0, "", nil},
			PayloadType:        100,
		}, RTPCodecTypeVideo)
	}
	if p.EnableAv1 {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeAV1, 90000, 0, "profile=2;level-idx=8;tier=1", nil},
			PayloadType:        45,
		}, RTPCodecTypeVideo)
	}
	i := &interceptor.Registry{}
	if p.PublicIP != "" {
		ips := []string{p.PublicIP}
		if p.PublicIPv6 != "" {
			ips = append(ips, p.PublicIPv6)
		}
		p.s.SetNAT1To1IPs(ips, ICECandidateTypeHost)
	}
	ports, err := ParsePort2(p.Port)
	if err != nil {
		p.Error("webrtc port config error", "error", err, "port", p.Port)
		return err
	}

	switch v := ports.(type) {
	case TCPPort:
		tcpport := int(v)
		tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: tcpport,
		})
		if err != nil {
			p.Error("webrtc listener tcp", "error", err)
		}
		p.Info("webrtc start listen", "port", tcpport)
		p.s.SetICETCPMux(NewICETCPMux(nil, tcpl, 4096))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeTCP4, NetworkTypeTCP6})
	case UDPRangePort:
		p.s.SetEphemeralUDPPortRange(uint16(v[0]), uint16(v[1]))
	case UDPPort:
		// 创建共享WEBRTC端口 默认9000
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: int(v),
		})
		if err != nil {
			p.Error("webrtc listener udp", "error", err)
			return err
		}
		p.Info("webrtc start listen", "port", v)
		p.s.SetICEUDPMux(NewICEUDPMux(nil, udpListener))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
	}
	if err = RegisterDefaultInterceptors(&p.m, i); err != nil {
		return err
	}
	p.api = NewAPI(WithMediaEngine(&p.m),
		WithInterceptorRegistry(i), WithSettingEngine(p.s))
	return
}

func (*WebRTCPlugin) Test_Publish(w http.ResponseWriter, r *http.Request) {
	w.Write(publishHTML)
}
func (*WebRTCPlugin) Test_ScreenShare(w http.ResponseWriter, r *http.Request) {
	w.Write(publishHTML)
}
func (*WebRTCPlugin) Test_Subscribe(w http.ResponseWriter, r *http.Request) {
	w.Write(subscribeHTML)
}

// https://datatracker.ietf.org/doc/html/draft-ietf-wish-whip
func (conf *WebRTCPlugin) Push_(w http.ResponseWriter, r *http.Request) {
	streamPath := r.URL.Path[len("/push/"):]
	rawQuery := r.URL.RawQuery
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		auth = auth[len("Bearer "):]
		if rawQuery != "" {
			rawQuery += "&bearer=" + auth
		} else {
			rawQuery = "bearer=" + auth
		}
		conf.Info("push", "stream", streamPath, "bearer", auth)
	}
	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", "/webrtc/api/stop/push/"+streamPath)
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	bytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var conn Connection
	conn.SDP = string(bytes)
	if conn.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var publisher *m7s.Publisher
	if publisher, err = conf.Publish(streamPath, conn.PeerConnection, r.RemoteAddr); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conn.OnTrack(func(track *TrackRemote, receiver *RTPReceiver) {
		publisher.Info("OnTrack", "kind", track.Kind().String(), "payloadType", uint8(track.Codec().PayloadType))
		var n int
		var err error
		if codecP := track.Codec(); track.Kind() == RTPCodecTypeAudio {
			if !publisher.PubAudio {
				return
			}
			mem := util.NewScalableMemoryAllocator(1 << 12)
			defer mem.Recycle()
			frame := &mrtp.RTPAudio{}
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
					err = publisher.WriteAudio(frame)
					frame = &mrtp.RTPAudio{}
					frame.AddRecycleBytes(buf)
					frame.Packets = []*rtp.Packet{&packet}
					frame.RTPCodecParameters = &codecP
					frame.SetAllocator(mem)
				}
			}
		} else {
			if !publisher.PubVideo {
				return
			}
			var lastPLISent time.Time
			mem := util.NewScalableMemoryAllocator(1 << 12)
			defer mem.Recycle()
			frame := &mrtp.RTPVideo{}
			frame.RTPCodecParameters = &codecP
			frame.SetAllocator(mem)
			for {
				if time.Since(lastPLISent) > conf.PLI {
					if rtcpErr := conn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())}}); rtcpErr != nil {
						publisher.Error("writeRTCP", "error", rtcpErr)
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
					// t := time.Now()
					err = publisher.WriteVideo(frame)
					// fmt.Println("write video", time.Since(t))
					frame = &mrtp.RTPVideo{}
					frame.AddRecycleBytes(buf)
					frame.Packets = []*rtp.Packet{&packet}
					frame.RTPCodecParameters = &codecP
					frame.SetAllocator(mem)
				}
			}
		}
	})
	conn.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			publisher.Info(ice.ToJSON().Candidate)
		}
	})
	conn.OnDataChannel(func(d *DataChannel) {
		publisher.Info("OnDataChannel", "label", d.Label())
		d.OnMessage(func(msg DataChannelMessage) {
			conn.SDP = string(msg.Data[1:])
			publisher.Debug("dc message", "sdp", conn.SDP)
			if err := conn.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: conn.SDP}); err != nil {
				return
			}
			if answer, err := conn.GetAnswer(); err == nil {
				d.SendText(answer)
			} else {
				return
			}
			switch msg.Data[0] {
			case '0':
				publisher.Stop(errors.New("stop by remote"))
			case '1':

			}
		})
	})
	conn.OnConnectionStateChange(func(state PeerConnectionState) {
		publisher.Info("Connection State has changed:" + state.String())
		switch state {
		case PeerConnectionStateConnected:

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed, PeerConnectionStateClosed:
			publisher.Stop(errors.New("connection state:" + state.String()))
		}
	})
	if err := conn.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: conn.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if answer, err := conn.GetAnswer(); err == nil {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, answer)
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (conf *WebRTCPlugin) Play_(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/sdp")
	streamPath := r.URL.Path[len("/play/"):]
	rawQuery := r.URL.RawQuery
	var conn Connection
	bytes, err := io.ReadAll(r.Body)
	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}()
	if err != nil {
		return
	}
	conn.SDP = string(bytes)
	if conn.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
		return
	}
	var suber *m7s.Subscriber
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	if suber, err = conf.Subscribe(streamPath, conn.PeerConnection); err != nil {
		return
	}
	var useDC bool
	var audioTLSRTP, videoTLSRTP *TrackLocalStaticRTP
	var audioSender, videoSender *RTPSender
	if suber.Publisher != nil {
		if vt := suber.Publisher.VideoTrack.AVTrack; vt != nil {
			if vt.FourCC() == codec.FourCC_H265 {
				useDC = true
			} else {
				var rcc RTPCodecParameters
				if ctx, ok := vt.ICodecCtx.(mrtp.IRTPCtx); ok {
					rcc = ctx.GetRTPCodecParameter()
				} else {
					var rtpCtx mrtp.RTPData
					var tmpAVTrack AVTrack
					tmpAVTrack.ICodecCtx, tmpAVTrack.SequenceFrame, err = rtpCtx.ConvertCtx(vt.ICodecCtx)
					if err == nil {
						rcc = tmpAVTrack.ICodecCtx.(mrtp.IRTPCtx).GetRTPCodecParameter()
					} else {
						return
					}
				}
				videoTLSRTP, err = NewTrackLocalStaticRTP(rcc.RTPCodecCapability, vt.FourCC().String(), suber.StreamPath)
				if err != nil {
					return
				}
				videoSender, err = conn.PeerConnection.AddTrack(videoTLSRTP)
				if err != nil {
					return
				}
				go func() {
					rtcpBuf := make([]byte, 1500)
					for {
						if n, _, rtcpErr := videoSender.Read(rtcpBuf); rtcpErr != nil {
							suber.Warn("rtcp read error", "error", rtcpErr)
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
		}
		if at := suber.Publisher.AudioTrack.AVTrack; at != nil {
			if at.FourCC() == codec.FourCC_MP4A {
				useDC = true
			} else {
				ctx := at.ICodecCtx.(interface {
					GetRTPCodecCapability() RTPCodecCapability
				})
				audioTLSRTP, err = NewTrackLocalStaticRTP(ctx.GetRTPCodecCapability(), at.FourCC().String(), suber.StreamPath)
				if err != nil {
					return
				}
				audioSender, err = conn.PeerConnection.AddTrack(audioTLSRTP)
				if err != nil {
					return
				}
			}
		}
	}

	if conf.EnableDC && useDC {
		dc, err := conn.CreateDataChannel(suber.StreamPath, nil)
		if err != nil {
			return
		}
		go func() {
			// suber.Handle(m7s.SubscriberHandler{
			// 	OnAudio: func(audio *rtmp.RTMPAudio) error {
			// 	},
			// 	OnVideo: func(video *rtmp.RTMPVideo) error {
			// 	},
			// })
			dc.Close()
		}()
	} else {
		if audioSender == nil {
			suber.SubAudio = false
		}
		if videoSender == nil {
			suber.SubVideo = false
		}
		go m7s.PlayBlock(suber, func(frame *mrtp.RTPAudio) (err error) {
			for _, p := range frame.Packets {
				if err = audioTLSRTP.WriteRTP(p); err != nil {
					return
				}
			}
			return
		}, func(frame *mrtp.RTPVideo) error {
			for _, p := range frame.Packets {
				if err := videoTLSRTP.WriteRTP(p); err != nil {
					return err
				}
			}
			return nil
		})
	}
	conn.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			suber.Info(ice.ToJSON().Candidate)
		}
	})
	if err = conn.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: conn.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sdp, err := conn.GetAnswer(); err == nil {
		w.Write([]byte(sdp))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
