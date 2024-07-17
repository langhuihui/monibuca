package rtp

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"io"
	"strings"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	RTPH264Ctx struct {
		RTPCtx
		codec.H264Ctx
	}
	RTPH265Ctx struct {
		RTPCtx
		codec.H265Ctx
		DONL bool
	}
	RTPAV1Ctx struct {
		RTPCtx
		codec.AV1Ctx
	}
	RTPVP9Ctx struct {
		RTPCtx
	}
	RTPVideo struct {
		RTPData
	}
)

var (
	_ IAVFrame       = (*RTPVideo)(nil)
	_ IVideoCodecCtx = (*RTPH264Ctx)(nil)
	_ IVideoCodecCtx = (*RTPH265Ctx)(nil)
	_ IVideoCodecCtx = (*RTPAV1Ctx)(nil)
)

const (
	H265_NALU_AP = h265parser.NAL_UNIT_UNSPECIFIED_48
	H265_NALU_FU = h265parser.NAL_UNIT_UNSPECIFIED_49
	startBit     = 1 << 7
	endBit       = 1 << 6
	MTUSize      = 1460
)

func (r *RTPVideo) Parse(t *AVTrack) (err error) {
	switch r.MimeType {
	case webrtc.MimeTypeH264:
		var ctx *RTPH264Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*RTPH264Ctx)
		} else {
			ctx = &RTPH264Ctx{}
			ctx.parseFmtpLine(r.RTPCodecParameters)
			var sps, pps []byte
			//packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029
			if sprop, ok := ctx.Fmtp["sprop-parameter-sets"]; ok {
				if sprops := strings.Split(sprop, ","); len(sprops) == 2 {
					if sps, err = base64.StdEncoding.DecodeString(sprops[0]); err != nil {
						return
					}
					if pps, err = base64.StdEncoding.DecodeString(sprops[1]); err == nil {
						return
					}
				}
				if ctx.CodecData, err = h264parser.NewCodecDataFromSPSAndPPS(sps, pps); err != nil {
					return
				}
			}
			t.ICodecCtx = ctx
		}
		if t.Value.Raw, err = r.Demux(ctx); err != nil {
			return
		}
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case h264parser.NALU_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h264parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h264parser.NALU_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
			case codec.NALU_IDR_Picture:
				t.Value.IDR = true
			}
		}
	case webrtc.MimeTypeVP9:
		// var ctx RTPVP9Ctx
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypeAV1:
		// var ctx RTPAV1Ctx
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypeH265:
		var ctx *RTPH265Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*RTPH265Ctx)
		} else {
			ctx = &RTPH265Ctx{}
			ctx.parseFmtpLine(r.RTPCodecParameters)
			var vps, sps, pps []byte
			if sprop_sps, ok := ctx.Fmtp["sprop-sps"]; ok {
				if sps, err = base64.StdEncoding.DecodeString(sprop_sps); err != nil {
					return
				}
			}
			if sprop_pps, ok := ctx.Fmtp["sprop-pps"]; ok {
				if pps, err = base64.StdEncoding.DecodeString(sprop_pps); err != nil {
					return
				}
			}
			if sprop_vps, ok := ctx.Fmtp["sprop-vps"]; ok {
				if vps, err = base64.StdEncoding.DecodeString(sprop_vps); err != nil {
					return
				}
			}
			if ctx.CodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps); err != nil {
				return
			}
			if sprop_donl, ok := ctx.Fmtp["sprop-max-don-diff"]; ok {
				if sprop_donl != "0" {
					ctx.DONL = true
				}
			}
			t.ICodecCtx = ctx
		}
		if t.Value.Raw, err = r.Demux(ctx); err != nil {
			return
		}
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				ctx = &RTPH265Ctx{}
				ctx.RecordInfo.VPS = [][]byte{nalu.ToBytes()}
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case h265parser.NAL_UNIT_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h265parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h265parser.NAL_UNIT_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				t.Value.IDR = true
			}
		}
	default:
		err = ErrUnsupportCodec
	}
	return
}

func (h264 *RTPH264Ctx) GetInfo() string {
	return h264.SDPFmtpLine
}

func (h265 *RTPH265Ctx) GetInfo() string {
	return h265.SDPFmtpLine
}

func (av1 *RTPAV1Ctx) GetInfo() string {
	return av1.SDPFmtpLine
}

func (r *RTPVideo) GetCTS() time.Duration {
	return 0
}

func (r *RTPVideo) Mux(codecCtx codec.ICodecCtx, from *AVFrame) {
	pts := uint32((from.Timestamp + from.CTS) * 90 / time.Millisecond)
	switch c := codecCtx.(type) {
	case *RTPH264Ctx:
		ctx := &c.RTPCtx
		r.RTPCodecParameters = &ctx.RTPCodecParameters
		var lastPacket *rtp.Packet
		if from.IDR && len(c.RecordInfo.SPS) > 0 && len(c.RecordInfo.PPS) > 0 {
			r.Append(ctx, pts, c.SPS())
			r.Append(ctx, pts, c.PPS())
		}
		for _, nalu := range from.Raw.(Nalus) {
			if reader := nalu.NewReader(); reader.Length > MTUSize {
				payloadLen := MTUSize
				if reader.Length+1 < payloadLen {
					payloadLen = reader.Length + 1
				}
				//fu-a
				mem := r.NextN(payloadLen)
				reader.ReadBytesTo(mem[1:])
				fuaHead, naluType := codec.NALU_FUA.Or(mem[1]&0x60), mem[1]&0x1f
				mem[0], mem[1] = fuaHead, naluType|startBit
				lastPacket = r.Append(ctx, pts, mem)
				for payloadLen = MTUSize; reader.Length > 0; lastPacket = r.Append(ctx, pts, mem) {
					if reader.Length+2 < payloadLen {
						payloadLen = reader.Length + 2
					}
					mem = r.NextN(payloadLen)
					reader.ReadBytesTo(mem[2:])
					mem[0], mem[1] = fuaHead, naluType
				}
				lastPacket.Payload[1] |= endBit
			} else {
				mem := r.NextN(reader.Length)
				reader.ReadBytesTo(mem)
				lastPacket = r.Append(ctx, pts, mem)
			}
		}
		lastPacket.Header.Marker = true
	case *RTPH265Ctx:
		ctx := &c.RTPCtx
		r.RTPCodecParameters = &ctx.RTPCodecParameters
		var lastPacket *rtp.Packet
		if from.IDR && len(c.RecordInfo.SPS) > 0 && len(c.RecordInfo.PPS) > 0 && len(c.RecordInfo.VPS) > 0 {
			r.Append(ctx, pts, c.VPS())
			r.Append(ctx, pts, c.SPS())
			r.Append(ctx, pts, c.PPS())
		}
		for _, nalu := range from.Raw.(Nalus) {
			if reader := nalu.NewReader(); reader.Length > MTUSize {
				var b0, b1 byte
				_ = reader.ReadByteTo(&b0, &b1)
				//fu
				naluType := byte(codec.ParseH265NALUType(b0))
				b0 = (byte(H265_NALU_FU) << 1) | (b0 & 0b10000001)

				payloadLen := MTUSize
				if reader.Length+3 < payloadLen {
					payloadLen = reader.Length + 3
				}
				mem := r.NextN(payloadLen)
				reader.ReadBytesTo(mem[3:])
				mem[0], mem[1], mem[2] = b0, b1, naluType|startBit
				lastPacket = r.Append(ctx, pts, mem)

				for payloadLen = MTUSize; reader.Length > 0; lastPacket = r.Append(ctx, pts, mem) {
					if reader.Length+3 < payloadLen {
						payloadLen = reader.Length + 3
					}
					mem = r.NextN(payloadLen)
					reader.ReadBytesTo(mem[3:])
					mem[0], mem[1], mem[2] = b0, b1, naluType
				}
				lastPacket.Payload[2] |= endBit
			} else {
				mem := r.NextN(reader.Length)
				reader.ReadBytesTo(mem)
				lastPacket = r.Append(ctx, pts, mem)
			}
		}
		lastPacket.Header.Marker = true
	}
}

func (r *RTPVideo) Demux(ictx codec.ICodecCtx) (any, error) {
	switch c := ictx.(type) {
	case *RTPH264Ctx:
		var nalus Nalus
		var nalu util.Memory
		var naluType codec.H264NALUType
		gotNalu := func() {
			if nalu.Size > 0 {
				nalus = append(nalus, nalu)
				nalu = util.Memory{}
			}
		}
		for _, packet := range r.Packets {
			b0 := packet.Payload[0]
			if t := codec.ParseH264NALUType(b0); t < 24 {
				nalu.AppendOne(packet.Payload)
				gotNalu()
			} else {
				offset := t.Offset()
				switch t {
				case codec.NALU_STAPA, codec.NALU_STAPB:
					if len(packet.Payload) <= offset {
						return nil, fmt.Errorf("invalid nalu size %d", len(packet.Payload))
					}
					for buffer := util.Buffer(packet.Payload[offset:]); buffer.CanRead(); {
						if nextSize := int(buffer.ReadUint16()); buffer.Len() >= nextSize {
							nalu.AppendOne(buffer.ReadN(nextSize))
							gotNalu()
						} else {
							return nil, fmt.Errorf("invalid nalu size %d", nextSize)
						}
					}
				case codec.NALU_FUA, codec.NALU_FUB:
					b1 := packet.Payload[1]
					if util.Bit1(b1, 0) {
						naluType.Parse(b1)
						nalu.AppendOne([]byte{naluType.Or(b0 & 0x60)})
					}
					if nalu.Size > 0 {
						nalu.AppendOne(packet.Payload[offset:])
					} else {
						return nil, errors.New("fu have no start")
					}
					if util.Bit1(b1, 1) {
						gotNalu()
					}
				default:
					return nil, fmt.Errorf("unsupported nalu type %d", t)
				}
			}
		}
		return nalus, nil
	case *RTPH265Ctx:
		var nalus Nalus
		var nalu util.Memory
		gotNalu := func() {
			if nalu.Size > 0 {
				nalus = append(nalus, nalu)
				nalu = util.Memory{}
			}
		}
		for _, packet := range r.Packets {
			b0 := packet.Payload[0]
			if t := codec.ParseH265NALUType(b0); t < H265_NALU_AP {
				nalu.AppendOne(packet.Payload)
				gotNalu()
			} else {
				var buffer = util.Buffer(packet.Payload)
				switch t {
				case H265_NALU_AP:
					buffer.ReadUint16()
					if c.DONL {
						buffer.ReadUint16()
					}
					for buffer.CanRead() {
						nalu.AppendOne(buffer.ReadN(int(buffer.ReadUint16())))
						gotNalu()
					}
					if c.DONL {
						buffer.ReadByte()
					}
				case H265_NALU_FU:
					if buffer.Len() < 3 {
						return nil, io.ErrShortBuffer
					}
					first3 := buffer.ReadN(3)
					fuHeader := first3[2]
					if c.DONL {
						buffer.ReadUint16()
					}
					if naluType := fuHeader & 0b00111111; util.Bit1(fuHeader, 0) {
						nalu.AppendOne([]byte{first3[0]&0b10000001 | (naluType << 1), first3[1]})
					}
					nalu.AppendOne(buffer)
					if util.Bit1(fuHeader, 1) {
						gotNalu()
					}
				default:
					return nil, fmt.Errorf("unsupported nalu type %d", t)
				}
			}
		}
		return nalus, nil
	}
	return nil, nil
}
