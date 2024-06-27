package rtp

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
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
	_        IAVFrame       = (*RTPVideo)(nil)
	_        IVideoCodecCtx = (*RTPH264Ctx)(nil)
	_        IVideoCodecCtx = (*RTPH265Ctx)(nil)
	_        IVideoCodecCtx = (*RTPAV1Ctx)(nil)
	spropReg                = regexp.MustCompile(`sprop-parameter-sets=(.+),([^;]+)(;|$)`)
)

const (
	startBit = 1 << 7
	endBit   = 1 << 6
	MTUSize  = 1460
)

func (r *RTPVideo) Parse(t *AVTrack) (isIDR, isSeq bool, raw any, err error) {
	switch r.MimeType {
	case webrtc.MimeTypeH264:
		var ctx *RTPH264Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*RTPH264Ctx)
		} else {
			ctx = &RTPH264Ctx{}
			//packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029
			ctx.RTPCodecParameters = *r.RTPCodecParameters
			if match := spropReg.FindStringSubmatch(ctx.SDPFmtpLine); len(match) > 2 {
				if sps, err := base64.StdEncoding.DecodeString(match[1]); err == nil {
					ctx.SPS = [][]byte{sps}
				}
				if pps, err := base64.StdEncoding.DecodeString(match[2]); err == nil {
					ctx.PPS = [][]byte{pps}
				}
			}
			t.ICodecCtx = ctx
		}
		raw, err = r.ToRaw(ctx)
		if err != nil {
			return
		}
		nalus := raw.(Nalus)
		for _, nalu := range nalus.Nalus {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case codec.NALU_SPS:
				ctx = &RTPH264Ctx{}
				ctx.SPS = [][]byte{nalu.ToBytes()}
				if err = ctx.SPSInfo.Unmarshal(ctx.SPS[0]); err != nil {
					return
				}
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case codec.NALU_PPS:
				ctx.PPS = [][]byte{nalu.ToBytes()}
			case codec.NALU_IDR_Picture:
				isIDR = true
			}
		}
	case webrtc.MimeTypeVP9:
		// var ctx RTPVP9Ctx
		// ctx.FourCC = codec.FourCC_VP9
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypeAV1:
		// var ctx RTPAV1Ctx
		// ctx.FourCC = codec.FourCC_AV1
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypeH265:
		var ctx *RTPH265Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*RTPH265Ctx)
		} else {
			ctx = &RTPH265Ctx{}
			ctx.RTPCodecParameters = *r.RTPCodecParameters
			t.ICodecCtx = ctx
		}
		raw, err = r.ToRaw(ctx)
		if err != nil {
			return
		}
		nalus := raw.(Nalus)
		for _, nalu := range nalus.Nalus {
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
				isIDR = true
			}
		}
	case "audio/MPEG4-GENERIC", "audio/AAC":
		var ctx RTPAACCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
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

func (h264 *RTPH264Ctx) CreateFrame(from *AVFrame) (frame IAVFrame, err error) {
	var r RTPVideo
	r.RTPCodecParameters = &h264.RTPCodecParameters
	if len(from.Wraps) > 0 {
		r.ScalableMemoryAllocator = from.Wraps[0].GetScalableMemoryAllocator()
	}
	nalus := from.Raw.(Nalus)
	var lastPacket *rtp.Packet
	createPacket := func(payload []byte) *rtp.Packet {
		h264.SequenceNumber++
		lastPacket = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SequenceNumber: h264.SequenceNumber,
				Timestamp:      uint32(nalus.PTS),
				SSRC:           h264.SSRC,
				PayloadType:    uint8(h264.PayloadType),
			},
			Payload: payload,
		}
		return lastPacket
	}
	if nalus.H264Type() == codec.NALU_IDR_Picture && len(h264.SPS) > 0 && len(h264.PPS) > 0 {
		r.Packets = append(r.Packets, createPacket(h264.SPS[0]), createPacket(h264.PPS[0]))
	}
	for _, nalu := range nalus.Nalus {
		if reader := nalu.NewReader(); reader.Length > MTUSize {
			//fu-a
			mem := r.Malloc(MTUSize)
			n := reader.ReadBytesTo(mem[1:])
			fuaHead := codec.NALU_FUA.Or(mem[1] & 0x60)
			mem[0] = fuaHead
			naluType := mem[1] & 0x1f
			mem[1] = naluType | startBit
			r.FreeRest(&mem, n+1)
			r.AddRecycleBytes(mem)
			r.Packets = append(r.Packets, createPacket(mem))
			for reader.Length > 0 {
				mem = r.Malloc(MTUSize)
				n = reader.ReadBytesTo(mem[2:])
				mem[0] = fuaHead
				mem[1] = naluType
				r.FreeRest(&mem, n+2)
				r.AddRecycleBytes(mem)
				r.Packets = append(r.Packets, createPacket(mem))
			}
			lastPacket.Payload[1] |= endBit
		} else {
			mem := r.NextN(reader.Length)
			reader.ReadBytesTo(mem)
			r.Packets = append(r.Packets, createPacket(mem))
		}
	}
	frame = &r
	lastPacket.Header.Marker = true
	return
}

func (r *RTPVideo) ToRaw(ictx ICodecCtx) (any, error) {
	switch ictx.(type) {
	case *RTPH264Ctx:
		var nalus Nalus
		var nalu util.Memory
		var naluType codec.H264NALUType
		gotNalu := func() {
			if nalu.Size > 0 {
				nalus.Nalus = append(nalus.Nalus, nalu)
				nalu = util.Memory{}
			}
		}
		for _, packet := range r.Packets {
			nalus.PTS = time.Duration(packet.Timestamp)
			// TODO: B-frame
			nalus.DTS = nalus.PTS
			b0 := packet.Payload[0]
			if t := codec.ParseH264NALUType(b0); t < 24 {
				nalu.Append(packet.Payload)
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
							nalu.Append(buffer.ReadN(nextSize))
							gotNalu()
						} else {
							return nil, fmt.Errorf("invalid nalu size %d", nextSize)
						}
					}
				case codec.NALU_FUA, codec.NALU_FUB:
					b1 := packet.Payload[1]
					if util.Bit1(b1, 0) {
						naluType.Parse(b1)
						nalu.Append([]byte{naluType.Or(b0 & 0x60)})
					}
					if nalu.Size > 0 {
						nalu.Append(packet.Payload[offset:])
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
