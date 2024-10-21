package plugin_webrtc

import (
	"fmt"
	"github.com/pion/rtcp"
	. "github.com/pion/webrtc/v3"
	"io"
	"m7s.live/v5"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	mrtp "m7s.live/v5/plugin/rtp/pkg"
	. "m7s.live/v5/plugin/webrtc/pkg"
	"net/http"
	"strings"
)

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
	conn := Connection{
		PLI: conf.PLI,
		SDP: string(bytes),
	}
	conn.Logger = conf.Logger
	if conn.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		ICEServers: conf.ICEServers,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if conn.Publisher, err = conf.Publish(conf.Context, streamPath); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conf.AddTask(&conn)
	conn.Receive()
	if err := conn.SetRemoteDescription(SessionDescription{Type: SDPTypeOffer, SDP: conn.SDP}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if answer, err := conn.GetAnswer(); err == nil {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, answer.SDP)
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
	conf.AddTask(&conn)
	var suber *m7s.Subscriber
	if rawQuery != "" {
		streamPath += "?" + rawQuery
	}
	if suber, err = conf.Subscribe(conf.Context, streamPath); err != nil {
		return
	}
	conn.AddTask(suber)
	var useDC bool
	var audioTLSRTP, videoTLSRTP *TrackLocalStaticRTP
	var audioSender, videoSender *RTPSender
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
				tmpAVTrack.ICodecCtx, _, err = rtpCtx.ConvertCtx(vt.ICodecCtx)
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

	if conf.EnableDC && useDC {
		dc, err := conn.CreateDataChannel(suber.StreamPath, nil)
		if err != nil {
			return
		}
		// TODO: DataChannel
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
		go m7s.PlayBlock(suber, func(frame *mrtp.Audio) (err error) {
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
		w.Write([]byte(sdp.SDP))
	} else {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
