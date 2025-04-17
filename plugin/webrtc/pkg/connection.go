package webrtc

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
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
	Publisher *m7s.Publisher
	SDP       string
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
		IO.Receive()
	}
	if IO.Subscriber != nil {
		IO.Depend(IO.Subscriber)
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

// H264CodecParams represents the parameters for an H.264 codec
type H264CodecParams struct {
	ProfileLevelID        string
	PacketizationMode     string
	LevelAsymmetryAllowed string
	SpropParameterSets    string
	OtherParams           map[string]string
}

// parseH264Params parses H.264 codec parameters from an fmtp line
func parseH264Params(fmtpLine string) H264CodecParams {
	params := H264CodecParams{
		OtherParams: make(map[string]string),
	}

	// Split the fmtp line into key-value pairs
	kvPairs := strings.Split(fmtpLine, ";")
	for _, kv := range kvPairs {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}

		parts := strings.SplitN(kv, "=", 2)
		key := strings.TrimSpace(parts[0])
		var value string
		if len(parts) > 1 {
			value = strings.TrimSpace(parts[1])
		}

		switch key {
		case "profile-level-id":
			params.ProfileLevelID = value
		case "packetization-mode":
			params.PacketizationMode = value
		case "level-asymmetry-allowed":
			params.LevelAsymmetryAllowed = value
		case "sprop-parameter-sets":
			params.SpropParameterSets = value
		default:
			params.OtherParams[key] = value
		}
	}

	return params
}

// extractH264CodecParams extracts all H.264 codec parameters from an SDP
func extractH264CodecParams(sdp string) []H264CodecParams {
	var result []H264CodecParams

	// Find all fmtp lines for H.264 codecs
	// First, find all a=rtpmap lines for H.264
	rtpmapRegex := regexp.MustCompile(`a=rtpmap:(\d+) H264/\d+`)
	rtpmapMatches := rtpmapRegex.FindAllStringSubmatch(sdp, -1)

	for _, rtpmapMatch := range rtpmapMatches {
		if len(rtpmapMatch) < 2 {
			continue
		}

		// Get the payload type
		payloadType := rtpmapMatch[1]

		// Find the corresponding fmtp line
		fmtpRegex := regexp.MustCompile(`a=fmtp:` + payloadType + ` ([^\r\n]+)`)
		fmtpMatch := fmtpRegex.FindStringSubmatch(sdp)

		if len(fmtpMatch) >= 2 {
			// Parse the fmtp line
			params := parseH264Params(fmtpMatch[1])
			result = append(result, params)
		}
	}

	return result
}

// findClosestProfileLevelID finds the closest matching profile-level-id
func findClosestProfileLevelID(availableIDs []string, currentID string) string {
	// If current ID is empty, return the first available one
	if currentID == "" && len(availableIDs) > 0 {
		return availableIDs[0]
	}

	// If current ID is in the available ones, use it
	for _, id := range availableIDs {
		if strings.EqualFold(id, currentID) {
			return currentID
		}
	}

	// Try to match the profile part (first two characters)
	if len(currentID) >= 2 {
		currentProfile := currentID[:2]
		for _, id := range availableIDs {
			if len(id) >= 2 && strings.EqualFold(id[:2], currentProfile) {
				return id
			}
		}
	}

	// If no match found, return the first available one
	if len(availableIDs) > 0 {
		return availableIDs[0]
	}

	// Fallback to the current one
	return currentID
}

// findBestMatchingH264Codec finds the best matching H.264 codec configuration
func findBestMatchingH264Codec(sdp string, currentFmtpLine string) string {
	// If no SDP or no current fmtp line, return the current one
	if sdp == "" || currentFmtpLine == "" {
		return currentFmtpLine
	}

	// Parse current parameters
	currentParams := parseH264Params(currentFmtpLine)

	// Extract all H.264 codec parameters from the SDP
	availableParams := extractH264CodecParams(sdp)

	// If no available parameters found, return the current one
	if len(availableParams) == 0 {
		return currentFmtpLine
	}

	// Extract all available profile-level-ids
	var availableProfileLevelIDs []string
	var packetizationModeMap = make(map[string]string)

	for _, params := range availableParams {
		if params.ProfileLevelID != "" {
			availableProfileLevelIDs = append(availableProfileLevelIDs, params.ProfileLevelID)
			// Store packetization mode for each profile-level-id
			if params.PacketizationMode != "" {
				packetizationModeMap[params.ProfileLevelID] = params.PacketizationMode
			}
		}
	}

	// Find the closest matching profile-level-id
	closestProfileLevelID := findClosestProfileLevelID(availableProfileLevelIDs, currentParams.ProfileLevelID)

	// Create result parameters
	resultParams := H264CodecParams{
		ProfileLevelID:        closestProfileLevelID,
		SpropParameterSets:    currentParams.SpropParameterSets, // Always use original sprop-parameter-sets
		LevelAsymmetryAllowed: "1",                              // Default to 1
	}

	// Use matching packetization mode if available
	if mode, ok := packetizationModeMap[closestProfileLevelID]; ok {
		resultParams.PacketizationMode = mode
	} else if currentParams.PacketizationMode != "" {
		resultParams.PacketizationMode = currentParams.PacketizationMode
	} else {
		resultParams.PacketizationMode = "1" // Default to 1
	}

	// Build and return the fmtp line
	return buildFmtpLine(resultParams)
}

// buildFmtpLine builds an fmtp line from H.264 codec parameters
func buildFmtpLine(params H264CodecParams) string {
	var parts []string

	// Add profile-level-id if present
	if params.ProfileLevelID != "" {
		parts = append(parts, "profile-level-id="+params.ProfileLevelID)
	}

	// Add packetization-mode if present
	if params.PacketizationMode != "" {
		parts = append(parts, "packetization-mode="+params.PacketizationMode)
	}

	// Add level-asymmetry-allowed if present
	if params.LevelAsymmetryAllowed != "" {
		parts = append(parts, "level-asymmetry-allowed="+params.LevelAsymmetryAllowed)
	}

	// Add sprop-parameter-sets if present
	if params.SpropParameterSets != "" {
		parts = append(parts, "sprop-parameter-sets="+params.SpropParameterSets)
	}

	// Add other parameters
	for k, v := range params.OtherParams {
		parts = append(parts, k+"="+v)
	}

	return strings.Join(parts, ";")
}

func (IO *MultipleConnection) SendSubscriber(subscriber *m7s.Subscriber) (audioSender, videoSender *RTPSender, err error) {
	var useDC bool
	var audioTLSRTP, videoTLSRTP *TrackLocalStaticRTP
	vctx, actx := subscriber.Publisher.GetVideoCodecCtx(), subscriber.Publisher.GetAudioCodecCtx()
	if IO.EnableDC {
		if IO.EnableDC && vctx != nil && vctx.FourCC() == codec.FourCC_H265 {
			useDC = true
		}
		if IO.EnableDC && actx != nil && actx.FourCC() == codec.FourCC_MP4A {
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

		// // For H.264, adjust codec parameters based on SDP
		// if rcc.MimeType == MimeTypeH264 && IO.SDP != "" {
		// 	// Find best matching codec configuration
		// 	originalFmtpLine := rcc.SDPFmtpLine
		// 	bestMatchingFmtpLine := findBestMatchingH264Codec(IO.SDP, rcc.SDPFmtpLine)

		// 	// Update the codec parameters if a better match was found
		// 	if bestMatchingFmtpLine != originalFmtpLine {
		// 		rcc.SDPFmtpLine = bestMatchingFmtpLine
		// 		IO.Info("Adjusted H.264 codec parameters", "from", originalFmtpLine, "to", bestMatchingFmtpLine)
		// 	}
		// }

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
	if actx != nil && !useDC {
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
	// // For H.264, adjust codec parameters based on SDP
	// if rcc.MimeType == MimeTypeH264 && r.pc.SDP != "" {
	// 	// Find best matching codec configuration
	// 	originalFmtpLine := rcc.SDPFmtpLine
	// 	bestMatchingFmtpLine := findBestMatchingH264Codec(r.pc.SDP, rcc.SDPFmtpLine)

	// 	// Update the codec parameters if a better match was found
	// 	if bestMatchingFmtpLine != originalFmtpLine {
	// 		rcc.SDPFmtpLine = bestMatchingFmtpLine
	// 		r.Info("Adjusted H.264 codec parameters", "from", originalFmtpLine, "to", bestMatchingFmtpLine)
	// 	}
	// }

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

func (c *SingleConnection) Start() (err error) {
	c.OnICECandidate(func(ice *ICECandidate) {
		if ice != nil {
			c.Info(ice.ToJSON().Candidate)
		}
	})
	// 监听ICE连接状态变化
	c.OnICEConnectionStateChange(func(state ICEConnectionState) {
		c.Debug("ICE connection state changed", "state", state.String())
		if state == ICEConnectionStateFailed {
			c.Error("ICE connection failed")
		}
	})

	c.OnConnectionStateChange(func(state PeerConnectionState) {
		c.Info("Connection State has changed:" + state.String())
		switch state {
		case PeerConnectionStateConnected:

		case PeerConnectionStateDisconnected, PeerConnectionStateFailed, PeerConnectionStateClosed:
			c.Stop(errors.New("connection state:" + state.String()))
		}
	})
	return
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
