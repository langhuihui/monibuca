package plugin_rtsp

import (
	"encoding/binary"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	mrtp "m7s.live/m7s/v5/plugin/rtp/pkg"
	. "m7s.live/m7s/v5/plugin/rtsp/pkg"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const defaultConfig = m7s.DefaultYaml(`tcp:
  listenaddr: :554`)

var _ = m7s.InstallPlugin[RTSPPlugin](defaultConfig)

type RTSPPlugin struct {
	m7s.Plugin
}

func (p *RTSPPlugin) OnTCPConnect(conn *net.TCPConn) {
	logger := p.Logger.With("remote", conn.RemoteAddr().String())
	var receiver *Receiver
	var err error
	nc := NewNetConnection(conn, logger)
	defer func() {
		nc.Destroy()
		if p := recover(); p != nil {
			err = p.(error)
			logger.Error(err.Error(), "stack", string(debug.Stack()))
		}
		if receiver != nil {
			receiver.Dispose(err)
		}
	}()
	var req *tcp.Request
	var timeout time.Duration
	var sendMode bool
	mem := util.NewScalableMemoryAllocator(1 << 12)
	defer mem.Recycle()
	for {
		req, err = nc.ReadRequest()
		if err != nil {
			return
		}

		if nc.URL == nil {
			nc.URL = req.URL
			logger = logger.With("url", nc.URL.String())
			nc.UserAgent = req.Header.Get("User-Agent")
			logger.Info("connect", "userAgent", nc.UserAgent)
		}

		//if !c.auth.Validate(req) {
		//	res := &tcp.Response{
		//		Status:  "401 Unauthorized",
		//		Header:  map[string][]string{"Www-Authenticate": {`Basic realm="go2rtc"`}},
		//		Request: req,
		//	}
		//	if err = c.WriteResponse(res); err != nil {
		//		return err
		//	}
		//	continue
		//}

		// Receiver: OPTIONS > DESCRIBE > SETUP... > PLAY > TEARDOWN
		// Sender: OPTIONS > ANNOUNCE > SETUP... > RECORD > TEARDOWN
		switch req.Method {
		case MethodOptions:
			res := &tcp.Response{
				Header: map[string][]string{
					"Public": {"OPTIONS, SETUP, TEARDOWN, DESCRIBE, PLAY, PAUSE, ANNOUNCE, RECORD"},
				},
				Request: req,
			}
			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodAnnounce:
			if req.Header.Get("Content-Type") != "application/sdp" {
				err = errors.New("wrong content type")
				return
			}

			nc.SDP = string(req.Body) // for info

			if nc.Medias, err = UnmarshalSDP(req.Body); err != nil {
				return
			}

			receiver = &Receiver{
				NetConnection: nc,
			}

			if receiver.Publisher, err = p.Publish(strings.TrimPrefix(nc.URL.Path, "/")); err != nil {
				receiver = nil
				err = nc.WriteResponse(&tcp.Response{
					StatusCode: 500, Status: err.Error(),
				})
				return
			}

			for i, media := range nc.Medias {
				if codec := media.Codecs[0]; codec.IsAudio() {
					receiver.AudioCodecParameters = &webrtc.RTPCodecParameters{
						RTPCodecCapability: webrtc.RTPCodecCapability{
							MimeType:     "audio/" + codec.Name,
							ClockRate:    codec.ClockRate,
							Channels:     codec.Channels,
							SDPFmtpLine:  codec.FmtpLine,
							RTCPFeedback: nil,
						},
						PayloadType: webrtc.PayloadType(codec.PayloadType),
					}
					receiver.AudioChannelID = byte(i) << 1
				} else if codec.IsVideo() {
					receiver.VideoChannelID = byte(i) << 1
					receiver.VideoCodecParameters = &webrtc.RTPCodecParameters{
						RTPCodecCapability: webrtc.RTPCodecCapability{
							MimeType:     "video/" + codec.Name,
							ClockRate:    codec.ClockRate,
							Channels:     codec.Channels,
							SDPFmtpLine:  codec.FmtpLine,
							RTCPFeedback: nil,
						},
						PayloadType: webrtc.PayloadType(codec.PayloadType),
					}
				}
			}

			timeout = time.Second * 15

			res := &tcp.Response{Request: req}
			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodDescribe:
			sendMode = true
			timeout = time.Second * 60

			//if c.Senders == nil {
			//	res := &tcp.Response{
			//		Status:  "404 Not Found",
			//		Request: req,
			//	}
			//	return c.WriteResponse(res)
			//}

			res := &tcp.Response{
				Header: map[string][]string{
					"Content-Type": {"application/sdp"},
				},
				Request: req,
			}

			// convert tracks to real output medias
			var medias []*core.Media

			//for i, track := range c.Senders {
			//	media := &core.Media{
			//		Kind:      core.GetKind(track.Codec.Name),
			//		Direction: core.DirectionRecvonly,
			//		Codecs:    []*core.Codec{track.Codec},
			//		ID:        "trackID=" + strconv.Itoa(i),
			//	}
			//	medias = append(medias, media)
			//}

			res.Body, err = core.MarshalSDP(nc.SessionName, medias)
			if err != nil {
				return
			}

			nc.SDP = string(res.Body) // for info

			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodSetup:
			tr := req.Header.Get("Transport")

			res := &tcp.Response{
				Header:  map[string][]string{},
				Request: req,
			}

			const transport = "RTP/AVP/TCP;unicast;interleaved="
			if strings.HasPrefix(tr, transport) {
				nc.Session = core.RandString(8, 10)

				if sendMode {
					//if i := reqTrackID(req); i >= 0 && i < len(c.Senders) {
					//	// mark sender as SETUP
					//	c.Senders[i].Media.ID = MethodSetup
					//	tr = fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", i*2, i*2+1)
					//	res.Header.Set("Transport", tr)
					//} else {
					//	res.Status = "400 Bad Request"
					//}
				} else {
					res.Header.Set("Transport", tr[:len(transport)+3])
				}
			} else {
				res.Status = "461 Unsupported transport"
			}

			if err = nc.WriteResponse(res); err != nil {
				return
			}

		case MethodRecord, MethodPlay:
			if sendMode {
				// stop unconfigured senders
				//for _, track := range c.Senders {
				//	if track.Media.ID != MethodSetup {
				//		track.Close()
				//	}
				//}
			}

			res := &tcp.Response{Request: req}
			err = nc.WriteResponse(res)
			audioFrame := &mrtp.RTPAudio{}
			audioFrame.ScalableMemoryAllocator = mem
			audioFrame.RTPCodecParameters = receiver.AudioCodecParameters
			videoFrame := &mrtp.RTPVideo{}
			videoFrame.ScalableMemoryAllocator = mem
			videoFrame.RTPCodecParameters = receiver.VideoCodecParameters
			for err == nil {
				ts := time.Now()

				if err = conn.SetReadDeadline(ts.Add(timeout)); err != nil {
					return
				}
				var magic []byte
				// we can read:
				// 1. RTP interleaved: `$` + 1B channel number + 2B size
				// 2. RTSP response:   RTSP/1.0 200 OK
				// 3. RTSP request:    OPTIONS ...
				magic, err = nc.Peek(4)
				if err != nil {
					return
				}

				var channelID byte
				var size int
				var buf []byte
				if magic[0] != '$' {
					logger.Warn("not magic")
					switch string(magic) {
					case "RTSP":
						var res *tcp.Response
						if res, err = nc.ReadResponse(); err != nil {
							return
						}
						logger.Warn(string(res.Body))
						// for playing backchannel only after OK response on play

						continue

					case "OPTI", "TEAR", "DESC", "SETU", "PLAY", "PAUS", "RECO", "ANNO", "GET_", "SET_":
						var req *tcp.Request
						if req, err = nc.ReadRequest(); err != nil {
							return
						}

						if req.Method == MethodOptions {
							res := &tcp.Response{Request: req}
							if err = nc.WriteResponse(res); err != nil {
								return
							}
						}
						continue

					default:
						logger.Error("wrong input")
						//c.Fire("RTSP wrong input")
						//
						//for i := 0; ; i++ {
						//	// search next start symbol
						//	if _, err = c.reader.ReadBytes('$'); err != nil {
						//		return err
						//	}
						//
						//	if channelID, err = c.reader.ReadByte(); err != nil {
						//		return err
						//	}
						//
						//	// TODO: better check maximum good channel ID
						//	if channelID >= 20 {
						//		continue
						//	}
						//
						//	buf4 = make([]byte, 2)
						//	if _, err = io.ReadFull(c.reader, buf4); err != nil {
						//		return err
						//	}
						//
						//	// check if size good for RTP
						//	size = binary.BigEndian.Uint16(buf4)
						//	if size <= 1500 {
						//		break
						//	}
						//
						//	// 10 tries to find good packet
						//	if i >= 10 {
						//		return fmt.Errorf("RTSP wrong input")
						//	}
						//}
					}
				} else {
					// hope that the odd channels are always RTCP

					channelID = magic[1]

					// get data size
					size = int(binary.BigEndian.Uint16(magic[2:]))
					// skip 4 bytes from c.reader.Peek
					if err = nc.Skip(4); err != nil {
						return
					}
					buf = mem.Malloc(size)
					if err = nc.ReadNto(size, buf); err != nil {
						return
					}
				}

				if channelID&1 == 0 {
					switch channelID {
					case receiver.AudioChannelID:
						if !receiver.PubAudio {
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
							err = receiver.WriteAudio(audioFrame)
							audioFrame = &mrtp.RTPAudio{}
							audioFrame.AddRecycleBytes(buf)
							audioFrame.Packets = []*rtp.Packet{packet}
							audioFrame.RTPCodecParameters = receiver.VideoCodecParameters
							audioFrame.ScalableMemoryAllocator = mem
						}
					case receiver.VideoChannelID:
						if !receiver.PubVideo {
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
							err = receiver.WriteVideo(videoFrame)
							// fmt.Println("write video", time.Since(t))
							videoFrame = &mrtp.RTPVideo{}
							videoFrame.AddRecycleBytes(buf)
							videoFrame.Packets = []*rtp.Packet{packet}
							videoFrame.RTPCodecParameters = receiver.VideoCodecParameters
							videoFrame.ScalableMemoryAllocator = mem
						}
					default:

					}
				} else {
					msg := &RTCP{Channel: channelID}
					mem.Free(buf)
					if err = msg.Header.Unmarshal(buf); err != nil {
						return
					}
					if msg.Packets, err = rtcp.Unmarshal(buf); err != nil {
						return
					}
					logger.Debug("rtcp", "type", msg.Header.Type, "length", msg.Header.Length)
					// TODO: rtcp msg
				}

				//if keepaliveDT != 0 && ts.After(keepaliveTS) {
				//	req := &tcp.Request{Method: MethodOptions, URL: c.URL}
				//	if err = c.WriteRequest(req); err != nil {
				//		return
				//	}
				//
				//	keepaliveTS = ts.Add(keepaliveDT)
				//}
			}
			return

		case MethodTeardown:
			res := &tcp.Response{Request: req}
			_ = nc.WriteResponse(res)
			return

		default:
			p.Warn("unsupported method", "method", req.Method)
		}
	}
}

func reqTrackID(req *tcp.Request) int {
	var s string
	if req.URL.RawQuery != "" {
		s = req.URL.RawQuery
	} else {
		s = req.URL.Path
	}
	if i := strings.LastIndexByte(s, '='); i > 0 {
		if i, err := strconv.Atoi(s[i+1:]); err == nil {
			return i
		}
	}
	return -1
}
