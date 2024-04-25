package rtp

import (
	"fmt"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type RTPData struct {
	*webrtc.RTPCodecParameters
	Packets []*rtp.Packet
	util.RecyclableMemory
}

func (r *RTPData) String() string {
	return fmt.Sprintf("RTPData{Packets: %d, Codec: %s}", len(r.Packets), r.MimeType)
}

func (r *RTPData) GetTimestamp() time.Duration {
	return time.Duration(r.Packets[0].Timestamp) * time.Second / time.Duration(r.ClockRate)
}

func (r *RTPData) GetSize() (s int) {
	for _, p := range r.Packets {
		s += p.MarshalSize()
	}
	return
}

func (r *RTPData) IsIDR() bool {
	return false
}

type (
	RTPCtx struct {
		codec.FourCC
		webrtc.RTPCodecParameters
		SequenceNumber uint16
		SSRC           uint32
	}
	RTPG711Ctx struct {
		RTPCtx
	}
	RTPOPUSCtx struct {
		RTPCtx
	}
	RTPAACCtx struct {
		RTPCtx
	}
)

func (r *RTPCtx) GetRTPCodecCapability() webrtc.RTPCodecCapability {
	return r.RTPCodecCapability
}

func (r *RTPCtx) GetSequenceFrame() IAVFrame {
	return nil
}

func (r *RTPCtx) Parse(ICodecCtx) error {
	panic("unimplemented")
}

func (r *RTPData) DecodeConfig(from ICodecCtx) (codecCtx ICodecCtx, err error) {
	if r == nil {
		switch fourCC := from.Codec(); fourCC {
		case codec.FourCC_H264:
			var ctx RTPH264Ctx
			ctx.FourCC = fourCC
			codecCtx = &ctx
		case codec.FourCC_H265:
			var ctx RTPH265Ctx
			ctx.FourCC = fourCC
			codecCtx = &ctx			
		}
		return
	}
	switch r.MimeType {
	case webrtc.MimeTypeOpus:
		var ctx RTPOPUSCtx
		ctx.FourCC = codec.FourCC_OPUS
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypePCMA:
		var ctx RTPG711Ctx
		ctx.FourCC = codec.FourCC_ALAW
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypePCMU:
		var ctx RTPG711Ctx
		ctx.FourCC = codec.FourCC_ULAW
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypeH264:
		var ctx RTPH264Ctx
		ctx.FourCC = codec.FourCC_H264
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypeVP9:
		var ctx RTPVP9Ctx
		ctx.FourCC = codec.FourCC_VP9
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypeAV1:
		var ctx RTPAV1Ctx
		ctx.FourCC = codec.FourCC_AV1
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case webrtc.MimeTypeH265:
		var ctx RTPH265Ctx
		ctx.FourCC = codec.FourCC_H265
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	case "audio/MPEG4-GENERIC", "audio/AAC":
		var ctx RTPAACCtx
		ctx.FourCC = codec.FourCC_MP4A
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		codecCtx = &ctx
	default:
		err = ErrUnsupportCodec
	}
	return
}

type RTPAudio struct {
	RTPData
}

func (ctx *RTPCtx) CreateFrame(any) (IAVFrame, error) {
	panic("unimplemented")
}

func (r *RTPAudio) ToRaw(ICodecCtx) (any, error) {
	return nil, nil
}
