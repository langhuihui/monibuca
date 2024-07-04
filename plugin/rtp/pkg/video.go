package rtp

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
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
	startBit = 1 << 7
	endBit   = 1 << 6
	MTUSize  = 1460
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
			//packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029
			if sprop, ok := ctx.Fmtp["sprop-parameter-sets"]; ok {
				if sprops := strings.Split(sprop, ","); len(sprops) == 2 {
					if sps, err := base64.StdEncoding.DecodeString(sprops[0]); err == nil {
						ctx.SPS = [][]byte{sps}
					}
					if pps, err := base64.StdEncoding.DecodeString(sprops[1]); err == nil {
						ctx.PPS = [][]byte{pps}
					}
				}
			}
			t.ICodecCtx = ctx
		}
		if t.Value.Raw, err = r.Demux(ctx); err != nil {
			return
		}
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case codec.NALU_SPS:
				ctx.SPS = [][]byte{nalu.ToBytes()}
				if err = ctx.SPSInfo.Unmarshal(ctx.SPS[0]); err != nil {
					return
				}
			case codec.NALU_PPS:
				ctx.PPS = [][]byte{nalu.ToBytes()}
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
			t.ICodecCtx = ctx
		}
		if t.Value.Raw, err = r.Demux(ctx); err != nil {
			return
		}
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case codec.NAL_UNIT_SPS:
				ctx = &RTPH265Ctx{}
				ctx.SPS = [][]byte{nalu.ToBytes()}
				ctx.SPSInfo.Unmarshal(ctx.SPS[0])
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case codec.NAL_UNIT_PPS:
				ctx.PPS = [][]byte{nalu.ToBytes()}
			case codec.NAL_UNIT_VPS:
				ctx.VPS = [][]byte{nalu.ToBytes()}
			case codec.NAL_UNIT_CODED_SLICE_BLA,
				codec.NAL_UNIT_CODED_SLICE_BLANT,
				codec.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				codec.NAL_UNIT_CODED_SLICE_IDR,
				codec.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				codec.NAL_UNIT_CODED_SLICE_CRA:
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
		if from.IDR && len(c.SPS) > 0 && len(c.PPS) > 0 {
			r.Append(ctx, pts, c.SPS[0])
			r.Append(ctx, pts, c.PPS[0])
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
	}
}

func (r *RTPVideo) Demux(ictx codec.ICodecCtx) (any, error) {
	switch ictx.(type) {
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
	}
	return nil, nil
}
