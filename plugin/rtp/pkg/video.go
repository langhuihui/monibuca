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
		}
		raw, err = r.ToRaw(ctx)
		if err != nil {
			return
		}
		nalus := raw.(Nalus)
		if len(nalus.Nalus) > 0 {
			isIDR = nalus.H264Type() == codec.NALU_IDR_Picture
		}
		t.ICodecCtx = ctx
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
		var ctx RTPH265Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
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
		reader := util.NewBuffersFromBytes(nalu...)
		startIndex := len(r.Packets)
		if reader.Length > 1460 {
			//fu-a
			for reader.Length > 0 {
				mem := r.Malloc(1460)
				n := reader.ReadBytesTo(mem[1:])
				mem[0] = codec.NALU_FUA.Or(mem[1] & 0x60)
				if n < 1459 {
					r.RecycleBack(1459 - n)
				}
				r.Packets = append(r.Packets, createPacket(mem))
			}
			r.Packets[startIndex].Payload[1] |= 1 << 7 // set start bit
			lastPacket.Payload[1] |= 1 << 6            // set end bit
		} else {
			mem := r.Malloc(reader.Length)
			reader.ReadBytesTo(mem)
			r.Packets = append(r.Packets, createPacket(mem))
		}
	}
	frame = &r
	lastPacket.Header.Marker = true
	return
}

func (r *RTPVideo) ToRaw(ictx ICodecCtx) (any, error) {
	switch ctx := ictx.(type) {
	case *RTPH264Ctx:
		var nalus Nalus
		var nalu Nalu
		var naluType codec.H264NALUType
		gotNalu := func(t codec.H264NALUType) {
			if len(nalu) > 0 {
				switch t {
				case codec.NALU_SPS:
					ctx.SPS = [][]byte{slices.Concat(nalu...)}
					ctx.SPSInfo.Unmarshal(ctx.SPS[0])
				case codec.NALU_PPS:
					ctx.PPS = [][]byte{slices.Concat(nalu...)}
				default:
					nalus.Nalus = append(nalus.Nalus, nalu)
				}
				nalu = nil
			}
		}
		for i := 0; i < len(r.Packets); i++ {
			packet := r.Packets[i]
			nalus.PTS = time.Duration(packet.Timestamp)
			nalus.DTS = nalus.PTS
			if t := codec.ParseH264NALUType(packet.Payload[0]); t < 24 {
				nalu = [][]byte{packet.Payload}
				gotNalu(t)
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
							gotNalu(codec.ParseH264NALUType(nalu[0][0]))
						} else {
							return nil, fmt.Errorf("invalid nalu size %d", nextSize)
						}
					}
				case codec.NALU_FUA, codec.NALU_FUB:
					b1 := packet.Payload[1]
					if util.Bit1(b1, 0) {
						naluType.Parse(b1)
						firstByte := naluType.Or(packet.Payload[0] & 0x60)
						nalu = append([][]byte{{firstByte}}, packet.Payload[offset:])
					}
					if len(nalu) > 0 {
						nalu = append(nalu, packet.Payload[offset:])
					} else {
						return nil, errors.New("fu have no start")
					}
					if util.Bit1(b1, 1) {
						gotNalu(naluType)
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
