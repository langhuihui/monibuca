package rtp

import (
	"errors"
	"fmt"
	"slices"
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

func (r *RTPVideo) Parse(t *AVTrack) (isIDR, isSeq bool, raw any, err error) {
	switch r.MimeType {
	case webrtc.MimeTypeH264:
		var ctx *RTPH264Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*RTPH264Ctx)
		} else {
			ctx = &RTPH264Ctx{}
			ctx.RTPCodecParameters = *r.RTPCodecParameters
			t.ICodecCtx = ctx
		}
		raw, err = r.ToRaw(ctx)
		if err != nil {
			return
		}
		nalus := raw.(Nalus)
		for _, nalu := range nalus.Nalus {
			switch codec.ParseH264NALUType(nalu[0][0]) {
			case codec.NALU_SPS:
				ctx = &RTPH264Ctx{}
				ctx.SPS = [][]byte{slices.Concat(nalu...)}
				ctx.SPSInfo.Unmarshal(ctx.SPS[0])
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case codec.NALU_PPS:
				ctx.PPS = [][]byte{slices.Concat(nalu...)}
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
			switch codec.ParseH265NALUType(nalu[0][0]) {
			case codec.NAL_UNIT_SPS:
				ctx = &RTPH265Ctx{}
				ctx.SPS = [][]byte{slices.Concat(nalu...)}
				ctx.SPSInfo.Unmarshal(ctx.SPS[0])
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case codec.NAL_UNIT_PPS:
				ctx.PPS = [][]byte{slices.Concat(nalu...)}
			case codec.NAL_UNIT_VPS:
				ctx.VPS = [][]byte{slices.Concat(nalu...)}
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
		// var ctx RTPAACCtx
		// ctx.FourCC = codec.FourCC_MP4A
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
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
	r.ScalableMemoryAllocator = from.Wraps[0].GetScalableMemoryAllocator()
	nalus := from.Raw.(Nalus)
	nalutype := nalus.H264Type()
	var lastPacket *rtp.Packet
	createPacket := func(payload []byte) *rtp.Packet {
		h264.SequenceNumber++
		lastPacket = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SequenceNumber: h264.SequenceNumber,
				Timestamp:      uint32(nalus.PTS),
				SSRC:           h264.SSRC,
				PayloadType:    96,
			},
			Payload: payload,
		}
		return lastPacket
	}
	if nalutype == codec.NALU_IDR_Picture {
		r.Packets = append(r.Packets, createPacket(h264.SPS[0]), createPacket(h264.PPS[0]))
	}
	for _, nalu := range nalus.Nalus {
		reader := util.NewReadableBuffersFromBytes(nalu...)
		if startIndex := len(r.Packets); reader.Length > 1460 {
			//fu-a
			for reader.Length > 0 {
				mem := r.NextN(1460)
				n := reader.ReadBytesTo(mem[1:])
				mem[0] = codec.NALU_FUA.Or(mem[1] & 0x60)
				if n < 1459 {
					r.Free(mem[n+1:])
					mem = mem[:n+1]
				}
				r.UpdateBuffer(-1, mem)
				r.Packets = append(r.Packets, createPacket(mem))
			}
			r.Packets[startIndex].Payload[1] |= 1 << 7 // set start bit
			lastPacket.Payload[1] |= 1 << 6            // set end bit
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
		var nalu Nalu
		var naluType codec.H264NALUType
		gotNalu := func() {
			if len(nalu) > 0 {
				nalus.Nalus = append(nalus.Nalus, nalu)
				nalu = nil
			}
		}
		for i := 0; i < len(r.Packets); i++ {
			packet := r.Packets[i]
			nalus.PTS = time.Duration(packet.Timestamp)
			nalus.DTS = nalus.PTS
			b0 := packet.Payload[0]
			if t := codec.ParseH264NALUType(b0); t < 24 {
				nalu = [][]byte{packet.Payload}
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
							nalu = [][]byte{buffer.ReadN(nextSize)}
							gotNalu()
						} else {
							return nil, fmt.Errorf("invalid nalu size %d", nextSize)
						}
					}
				case codec.NALU_FUA, codec.NALU_FUB:
					b1 := packet.Payload[1]
					if util.Bit1(b1, 0) {
						naluType.Parse(b1)
						nalu = [][]byte{{naluType.Or(b0 & 0x60)}}
					}
					if len(nalu) > 0 {
						nalu = append(nalu, packet.Payload[offset:])
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
