package rtp

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
	"unsafe"

	"github.com/bluenviron/mediacommon/pkg/bits"
	"github.com/deepch/vdk/codec/aacparser"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type RTPData struct {
	*webrtc.RTPCodecParameters
	Packets []*rtp.Packet
	util.RecyclableMemory
}

func (r *RTPData) Dump(t byte, w io.Writer) {
	m := r.GetAllocator().Borrow(3 + len(r.Packets)*2 + r.GetSize())
	m[0] = t
	binary.BigEndian.PutUint16(m[1:], uint16(len(r.Packets)))
	offset := 3
	for _, p := range r.Packets {
		size := p.MarshalSize()
		binary.BigEndian.PutUint16(m[offset:], uint16(size))
		offset += 2
		p.MarshalTo(m[offset:])
		offset += size
	}
	w.Write(m)
}

func (r *RTPData) String() (s string) {
	for _, p := range r.Packets {
		s += fmt.Sprintf("t: %d, s: %d, p: %02X %d\n", p.Timestamp, p.SequenceNumber, p.Payload[0:2], len(p.Payload))
	}
	return
}

func (r *RTPData) GetTimestamp() time.Duration {
	return time.Duration(r.Packets[0].Timestamp) * time.Second / time.Duration(r.ClockRate)
}

func (r *RTPData) GetCTS() time.Duration {
	return 0
}

func (r *RTPData) GetSize() (s int) {
	for _, p := range r.Packets {
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
		codec.PCMACtx
	}
	PCMUCtx struct {
		RTPCtx
		codec.PCMUCtx
	}
	OPUSCtx struct {
		RTPCtx
		codec.OPUSCtx
	}
	AACCtx struct {
		RTPCtx
		codec.AACCtx
		SizeLength       int // 通常为13
		IndexLength      int
		IndexDeltaLength int
	}
	IRTPCtx interface {
		GetRTPCodecParameter() webrtc.RTPCodecParameters
	}
)

func (r *RTPCtx) parseFmtpLine(cp *webrtc.RTPCodecParameters) {
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

func (r *RTPData) Append(ctx *RTPCtx, ts uint32, payload []byte) (lastPacket *rtp.Packet) {
	ctx.SequenceNumber++
	lastPacket = &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: ctx.SequenceNumber,
			Timestamp:      ts,
			SSRC:           ctx.SSRC,
			PayloadType:    uint8(ctx.PayloadType),
		},
		Payload: payload,
	}
	r.Packets = append(r.Packets, lastPacket)
	return
}

func (r *RTPData) ConvertCtx(from codec.ICodecCtx) (to codec.ICodecCtx, seq IAVFrame, err error) {
	switch from.FourCC() {
	case codec.FourCC_H264:
		var ctx H264Ctx
		ctx.H264Ctx = *from.GetBase().(*codec.H264Ctx)
		ctx.PayloadType = 96
		ctx.MimeType = webrtc.MimeTypeH264
		ctx.ClockRate = 90000
		spsInfo := ctx.SPSInfo
		ctx.SDPFmtpLine = fmt.Sprintf("sprop-parameter-sets=%s,%s;profile-level-id=%02x%02x%02x;level-asymmetry-allowed=1;packetization-mode=1", base64.StdEncoding.EncodeToString(ctx.SPS()), base64.StdEncoding.EncodeToString(ctx.PPS()), spsInfo.ProfileIdc, spsInfo.ConstraintSetFlag, spsInfo.LevelIdc)
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		to = &ctx
	case codec.FourCC_H265:
		var ctx H265Ctx
		ctx.H265Ctx = *from.GetBase().(*codec.H265Ctx)
		ctx.PayloadType = 98
		ctx.MimeType = webrtc.MimeTypeH265
		ctx.SDPFmtpLine = fmt.Sprintf("profile-id=1;sprop-sps=%s;sprop-pps=%s;sprop-vps=%s", base64.StdEncoding.EncodeToString(ctx.SPS()), base64.StdEncoding.EncodeToString(ctx.PPS()), base64.StdEncoding.EncodeToString(ctx.VPS()))
		ctx.ClockRate = 90000
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		to = &ctx
	case codec.FourCC_MP4A:
		var ctx AACCtx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		ctx.AACCtx = *from.GetBase().(*codec.AACCtx)
		ctx.MimeType = "audio/MPEG4-GENERIC"
		ctx.SDPFmtpLine = fmt.Sprintf("profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=%s", hex.EncodeToString(ctx.AACCtx.ConfigBytes))
		ctx.IndexLength = 3
		ctx.IndexDeltaLength = 3
		ctx.SizeLength = 13
		ctx.RTPCtx.Channels = uint16(ctx.AACCtx.GetChannels())
		ctx.PayloadType = 97
		ctx.ClockRate = uint32(ctx.CodecData.SampleRate())
		to = &ctx
	case codec.FourCC_ALAW:
		var ctx PCMACtx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		ctx.PCMACtx = *from.GetBase().(*codec.PCMACtx)
		ctx.MimeType = webrtc.MimeTypePCMA
		ctx.PayloadType = 8
		ctx.ClockRate = uint32(ctx.SampleRate)
		to = &ctx
	case codec.FourCC_ULAW:
		var ctx PCMUCtx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		ctx.PCMUCtx = *from.GetBase().(*codec.PCMUCtx)
		ctx.MimeType = webrtc.MimeTypePCMU
		ctx.PayloadType = 0
		ctx.ClockRate = uint32(ctx.SampleRate)
		to = &ctx
	case codec.FourCC_OPUS:
		var ctx OPUSCtx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		ctx.OPUSCtx = *from.GetBase().(*codec.OPUSCtx)
		ctx.MimeType = webrtc.MimeTypeOpus
		ctx.PayloadType = 111
		ctx.ClockRate = uint32(ctx.CodecData.SampleRate())
		to = &ctx
	}
	return
}

type Audio struct {
	RTPData
}

func (r *Audio) Parse(t *AVTrack) (err error) {
	switch r.MimeType {
	case webrtc.MimeTypeOpus:
		var ctx OPUSCtx
		ctx.parseFmtpLine(r.RTPCodecParameters)
		t.ICodecCtx = &ctx
	case webrtc.MimeTypePCMA:
		var ctx PCMACtx
		ctx.parseFmtpLine(r.RTPCodecParameters)
		t.ICodecCtx = &ctx
	case webrtc.MimeTypePCMU:
		var ctx PCMUCtx
		ctx.parseFmtpLine(r.RTPCodecParameters)
		t.ICodecCtx = &ctx
	case "audio/MP4A-LATM":
		var ctx *AACCtx
		if t.ICodecCtx != nil {
			// ctx = t.ICodecCtx.(*AACCtx)
		} else {
			ctx = &AACCtx{}
			ctx.parseFmtpLine(r.RTPCodecParameters)
			if conf, ok := ctx.Fmtp["config"]; ok {
				if ctx.AACCtx.ConfigBytes, err = hex.DecodeString(conf); err == nil {
					if ctx.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(ctx.AACCtx.ConfigBytes); err != nil {
						return
					}
				}
			}
			t.ICodecCtx = ctx
		}
	case "audio/MPEG4-GENERIC":
		var ctx *AACCtx
		if t.ICodecCtx != nil {
			// ctx = t.ICodecCtx.(*AACCtx)
		} else {
			ctx = &AACCtx{}
			ctx.parseFmtpLine(r.RTPCodecParameters)
			ctx.IndexLength = 3
			ctx.IndexDeltaLength = 3
			ctx.SizeLength = 13
			if conf, ok := ctx.Fmtp["config"]; ok {
				if ctx.AACCtx.ConfigBytes, err = hex.DecodeString(conf); err == nil {
					if ctx.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(ctx.AACCtx.ConfigBytes); err != nil {
						return
					}
				}
			}
			t.ICodecCtx = ctx
		}
	}
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

func (r *Audio) Demux(codexCtx codec.ICodecCtx) (any, error) {
	var data AudioData
	switch r.MimeType {
	case "audio/MP4A-LATM":
		var fragments util.Memory
		var fragmentsExpected int
		var fragmentsSize int
		for _, packet := range r.Packets {
			buf := packet.Payload
			if fragments.Size == 0 {
				pl, n, err := payloadLengthInfoDecode(buf)
				if err != nil {
					return nil, err
				}

				buf = buf[n:]
				bl := len(buf)

				if pl <= bl {
					data.AppendOne(buf[:pl])
					// there could be other data, due to otherDataPresent. Ignore it.
				} else {
					if pl > 5*1024 {
						fragments = util.Memory{} // discard pending fragments
						return nil, fmt.Errorf("access unit size (%d) is too big, maximum is %d",
							pl, 5*1024)
					}

					fragments.AppendOne(buf)
					fragmentsSize = pl
					fragmentsExpected = pl - bl
					continue
				}
			} else {
				bl := len(buf)

				if fragmentsExpected > bl {
					fragments.AppendOne(buf)
					fragmentsExpected -= bl
					continue
				}

				fragments.AppendOne(buf[:fragmentsExpected])
				// there could be other data, due to otherDataPresent. Ignore it.
				data.Append(fragments.Buffers...)
				if fragments.Size != fragmentsSize {
					return nil, fmt.Errorf("fragmented AU size is not correct %d != %d", data.Size, fragmentsSize)
				}
				fragments = util.Memory{}
			}
		}
	case "audio/MPEG4-GENERIC":
		var fragments util.Memory
		for _, packet := range r.Packets {
			if len(packet.Payload) < 2 {
				continue
			}
			auHeaderLen := util.ReadBE[int](packet.Payload[:2])
			if auHeaderLen == 0 {
				data.AppendOne(packet.Payload)
			} else {
				dataLens, err := r.readAUHeaders(codexCtx.(*AACCtx), packet.Payload[2:], auHeaderLen)
				if err != nil {
					return nil, err
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
								return nil, fmt.Errorf("invalid data len %d", dataLen)
							}
							data.AppendOne(payload[:dataLen])
							payload = payload[dataLen:]
						}
					} else {
						if len(dataLens) != 1 {
							return nil, fmt.Errorf("a fragmented packet can only contain one AU")
						}
						fragments.AppendOne(payload)
					}
				} else {
					if len(dataLens) != 1 {
						return nil, fmt.Errorf("a fragmented packet can only contain one AU")
					}
					fragments.AppendOne(payload)
					if !packet.Header.Marker {
						continue
					}
					if uint64(fragments.Size) != dataLens[0] {
						return nil, fmt.Errorf("fragmented AU size is not correct %d != %d", dataLens[0], fragments.Size)
					}
					data.Append(fragments.Buffers...)
					fragments = util.Memory{}
				}
			}
			break
		}
	default:
		for _, packet := range r.Packets {
			data.AppendOne(packet.Payload)
		}
	}
	return data, nil
}

func (r *Audio) Mux(codexCtx codec.ICodecCtx, from *AVFrame) {
	data := from.Raw.(AudioData)
	var ctx *RTPCtx
	var lastPacket *rtp.Packet
	switch c := codexCtx.(type) {
	case *AACCtx:
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
			reader.ReadBytesTo(mem[4:])
			lastPacket = r.Append(ctx, pts, mem)
		}
		lastPacket.Header.Marker = true
		return
	case *PCMACtx:
		ctx = &c.RTPCtx
	case *PCMUCtx:
		ctx = &c.RTPCtx
	}
	pts := uint32(from.Timestamp * time.Duration(ctx.ClockRate) / time.Second)
	if reader := data.NewReader(); reader.Length > MTUSize {
		for reader.Length > 0 {
			payloadLen := MTUSize
			if reader.Length < MTUSize {
				payloadLen = reader.Length
			}
			mem := r.NextN(payloadLen)
			reader.ReadBytesTo(mem)
			lastPacket = r.Append(ctx, pts, mem)
		}
	} else {
		mem := r.NextN(reader.Length)
		reader.ReadBytesTo(mem)
		lastPacket = r.Append(ctx, pts, mem)
	}
	lastPacket.Header.Marker = true
}

func (r *Audio) readAUHeaders(ctx *AACCtx, buf []byte, headersLen int) ([]uint64, error) {
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
