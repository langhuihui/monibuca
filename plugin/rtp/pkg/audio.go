package rtp

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"github.com/bluenviron/mediacommon/pkg/bits"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type RTPData struct {
	Sample
	Packets util.ReuseArray[rtp.Packet]
}

func (r *RTPData) Recycle() {
	r.RecyclableMemory.Recycle()
	r.Packets.Reset()
}

func (r *RTPData) String() (s string) {
	for p := range r.Packets.RangePoint {
		s += fmt.Sprintf("t: %d, s: %d, p: %02X %d\n", p.Timestamp, p.SequenceNumber, p.Payload[0:2], len(p.Payload))
	}
	return
}

func (r *RTPData) GetSize() (s int) {
	for p := range r.Packets.RangePoint {
		s += p.MarshalSize()
	}
	return
}

type (
	RTPCtx struct {
		webrtc.RTPCodecParameters
		Fmtp           map[string]string
		SequenceNumber uint16
		SSRC           uint32
	}
	PCMACtx struct {
		RTPCtx
		*codec.PCMACtx
	}
	PCMUCtx struct {
		RTPCtx
		*codec.PCMUCtx
	}
	OPUSCtx struct {
		RTPCtx
		*codec.OPUSCtx
	}
	AACCtx struct {
		RTPCtx
		*codec.AACCtx
		SizeLength       int // 通常为13
		IndexLength      int
		IndexDeltaLength int
	}
	IRTPCtx interface {
		GetRTPCodecParameter() webrtc.RTPCodecParameters
	}
)

func (r *RTPCtx) ParseFmtpLine(cp *webrtc.RTPCodecParameters) {
	r.RTPCodecParameters = *cp
	r.Fmtp = make(map[string]string)
	kvs := strings.Split(r.SDPFmtpLine, ";")
	for _, kv := range kvs {
		if kv = strings.TrimSpace(kv); kv == "" {
			continue
		}
		if key, value, found := strings.Cut(kv, "="); found {
			r.Fmtp[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
}

func (r *RTPCtx) GetInfo() string {
	return r.GetRTPCodecParameter().SDPFmtpLine
}
func (r *AACCtx) GetInfo() string {
	return r.AACCtx.GetInfo()
}
func (r *OPUSCtx) GetInfo() string {
	return r.OPUSCtx.GetInfo()
}
func (r *RTPCtx) GetRTPCodecParameter() webrtc.RTPCodecParameters {
	return r.RTPCodecParameters
}

func (r *RTPData) Append(ctx *RTPCtx, ts uint32, payload []byte) *rtp.Packet {
	ctx.SequenceNumber++
	r.Packets = append(r.Packets, rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: ctx.SequenceNumber,
			Timestamp:      ts,
			SSRC:           ctx.SSRC,
			PayloadType:    uint8(ctx.PayloadType),
		},
		Payload: payload,
	})
	return &r.Packets[len(r.Packets)-1]
}

var _ IAVFrame = (*AudioFrame)(nil)

type AudioFrame struct {
	RTPData
}

func (r *AudioFrame) Parse(data IAVFrame) (err error) {
	input := data.(*AudioFrame)
	r.Packets = append(r.Packets[:0], input.Packets...)
	return
}

func payloadLengthInfoDecode(buf []byte) (int, int, error) {
	lb := len(buf)
	l := 0
	n := 0

	for {
		if (lb - n) == 0 {
			return 0, 0, fmt.Errorf("not enough bytes")
		}

		b := buf[n]
		n++
		l += int(b)

		if b != 255 {
			break
		}
	}

	return l, n, nil
}

func (r *AudioFrame) Demux() (err error) {
	if len(r.Packets) == 0 {
		return ErrSkip
	}
	data := r.GetAudioData()
	// 从编解码器上下文获取 MimeType
	var mimeType string
	if rtpCtx, ok := r.ICodecCtx.(IRTPCtx); ok {
		mimeType = rtpCtx.GetRTPCodecParameter().MimeType
	}
	switch mimeType {
	case "audio/MP4A-LATM":
		var fragments util.Memory
		var fragmentsExpected int
		var fragmentsSize int
		for packet := range r.Packets.RangePoint {
			if len(packet.Payload) == 0 {
				continue
			}
			if packet.Padding {
				packet.Padding = false
			}
			buf := packet.Payload
			if fragments.Size == 0 {
				pl, n, err := payloadLengthInfoDecode(buf)
				if err != nil {
					return err
				}

				buf = buf[n:]
				bl := len(buf)

				if pl <= bl {
					data.PushOne(buf[:pl])
					// there could be other data, due to otherDataPresent. Ignore it.
				} else {
					if pl > 5*1024 {
						fragments = util.Memory{} // discard pending fragments
						return fmt.Errorf("access unit size (%d) is too big, maximum is %d",
							pl, 5*1024)
					}

					fragments.PushOne(buf)
					fragmentsSize = pl
					fragmentsExpected = pl - bl
					continue
				}
			} else {
				bl := len(buf)

				if fragmentsExpected > bl {
					fragments.PushOne(buf)
					fragmentsExpected -= bl
					continue
				}

				fragments.PushOne(buf[:fragmentsExpected])
				// there could be other data, due to otherDataPresent. Ignore it.
				data.Push(fragments.Buffers...)
				if fragments.Size != fragmentsSize {
					return fmt.Errorf("fragmented AU size is not correct %d != %d", data.Size, fragmentsSize)
				}
				fragments = util.Memory{}
			}
		}
	case "audio/MPEG4-GENERIC":
		var fragments util.Memory
		for packet := range r.Packets.RangePoint {
			if len(packet.Payload) < 2 {
				continue
			}
			auHeaderLen := util.ReadBE[int](packet.Payload[:2])
			if auHeaderLen == 0 {
				data.PushOne(packet.Payload)
			} else {
				dataLens, err := r.readAUHeaders(r.ICodecCtx.(*AACCtx), packet.Payload[2:], auHeaderLen)
				if err != nil {
					return err
				}
				payload := packet.Payload[2:]
				pos := auHeaderLen >> 3
				if (auHeaderLen % 8) != 0 {
					pos++
				}
				payload = payload[pos:]
				if fragments.Size == 0 {
					if packet.Marker {
						for _, dataLen := range dataLens {
							if len(payload) < int(dataLen) {
								return fmt.Errorf("invalid data len %d", dataLen)
							}
							data.PushOne(payload[:dataLen])
							payload = payload[dataLen:]
						}
					} else {
						if len(dataLens) != 1 {
							return fmt.Errorf("a fragmented packet can only contain one AU")
						}
						fragments.PushOne(payload)
					}
				} else {
					if len(dataLens) != 1 {
						return fmt.Errorf("a fragmented packet can only contain one AU")
					}
					fragments.PushOne(payload)
					if !packet.Header.Marker {
						continue
					}
					if uint64(fragments.Size) != dataLens[0] {
						return fmt.Errorf("fragmented AU size is not correct %d != %d", dataLens[0], fragments.Size)
					}
					data.Push(fragments.Buffers...)
					fragments = util.Memory{}
				}
			}
			break
		}
	default:
		for packet := range r.Packets.RangePoint {
			data.PushOne(packet.Payload)
		}
	}
	return nil
}

func (r *AudioFrame) Mux(from *Sample) (err error) {
	data := from.Raw.(*AudioData)
	var ctx *RTPCtx
	var lastPacket *rtp.Packet
	switch base := from.GetBase().(type) {
	case *codec.AACCtx:
		var c *AACCtx
		if r.ICodecCtx == nil {
			c = &AACCtx{}
			c.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
			c.AACCtx = base
			c.MimeType = "audio/MPEG4-GENERIC"
			c.SDPFmtpLine = fmt.Sprintf("profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=%s", hex.EncodeToString(c.ConfigBytes))
			c.IndexLength = 3
			c.IndexDeltaLength = 3
			c.SizeLength = 13
			c.RTPCtx.Channels = uint16(base.GetChannels())
			c.PayloadType = 97
			c.ClockRate = uint32(base.CodecData.SampleRate())
			r.ICodecCtx = c
		} else {
			c = r.ICodecCtx.(*AACCtx)
		}
		ctx = &c.RTPCtx
		pts := uint32(from.Timestamp * time.Duration(ctx.ClockRate) / time.Second)
		//AU_HEADER_LENGTH,因为单位是bit, 除以8就是auHeader的字节长度；又因为单个auheader字节长度2字节，所以再除以2就是auheader的个数。
		auHeaderLen := []byte{0x00, 0x10, (byte)((data.Size & 0x1fe0) >> 5), (byte)((data.Size & 0x1f) << 3)} // 3 = 16-13, 5 = 8-3
		for reader := data.NewReader(); reader.Length > 0; {
			payloadLen := MTUSize
			if reader.Length+4 < MTUSize {
				payloadLen = reader.Length + 4
			}
			mem := r.NextN(payloadLen)
			copy(mem, auHeaderLen)
			reader.Read(mem[4:])
			lastPacket = r.Append(ctx, pts, mem)
		}
		lastPacket.Header.Marker = true
		return
	case *codec.PCMACtx:
		if r.ICodecCtx == nil {
			var ctx PCMACtx
			ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
			ctx.PCMACtx = base
			ctx.MimeType = webrtc.MimeTypePCMA
			ctx.PayloadType = 8
			ctx.ClockRate = uint32(ctx.SampleRate)
			r.ICodecCtx = &ctx
		} else {
			ctx = &r.ICodecCtx.(*PCMACtx).RTPCtx
		}
	case *codec.PCMUCtx:
		if r.ICodecCtx == nil {
			var ctx PCMUCtx
			ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
			ctx.PCMUCtx = base
			ctx.MimeType = webrtc.MimeTypePCMU
			ctx.PayloadType = 0
			ctx.ClockRate = uint32(ctx.SampleRate)
			r.ICodecCtx = &ctx
		} else {
			ctx = &r.ICodecCtx.(*PCMUCtx).RTPCtx
		}
	}
	pts := uint32(from.Timestamp * time.Duration(ctx.ClockRate) / time.Second)
	if reader := data.NewReader(); reader.Length > MTUSize {
		for reader.Length > 0 {
			payloadLen := MTUSize
			if reader.Length < MTUSize {
				payloadLen = reader.Length
			}
			mem := r.NextN(payloadLen)
			reader.Read(mem)
			lastPacket = r.Append(ctx, pts, mem)
		}
	} else {
		mem := r.NextN(reader.Length)
		reader.Read(mem)
		lastPacket = r.Append(ctx, pts, mem)
	}
	lastPacket.Header.Marker = true
	return
}

func (r *AudioFrame) readAUHeaders(ctx *AACCtx, buf []byte, headersLen int) ([]uint64, error) {
	firstRead := false

	count := 0
	for i := 0; i < headersLen; {
		if i == 0 {
			i += ctx.SizeLength
			i += ctx.IndexLength
		} else {
			i += ctx.SizeLength
			i += ctx.IndexDeltaLength
		}
		count++
	}

	dataLens := make([]uint64, count)

	pos := 0
	i := 0

	for headersLen > 0 {
		dataLen, err := bits.ReadBits(buf, &pos, ctx.SizeLength)
		if err != nil {
			return nil, err
		}
		headersLen -= ctx.SizeLength

		if !firstRead {
			firstRead = true
			if ctx.IndexLength > 0 {
				auIndex, err := bits.ReadBits(buf, &pos, ctx.IndexLength)
				if err != nil {
					return nil, err
				}
				headersLen -= ctx.IndexLength

				if auIndex != 0 {
					return nil, fmt.Errorf("AU-index different than zero is not supported")
				}
			}
		} else if ctx.IndexDeltaLength > 0 {
			auIndexDelta, err := bits.ReadBits(buf, &pos, ctx.IndexDeltaLength)
			if err != nil {
				return nil, err
			}
			headersLen -= ctx.IndexDeltaLength

			if auIndexDelta != 0 {
				return nil, fmt.Errorf("AU-index-delta different than zero is not supported")
			}
		}

		dataLens[i] = dataLen
		i++
	}

	return dataLens, nil
}
