package rtp

import (
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type RTPData struct {
	*webrtc.RTPCodecParameters
	rtp.Packet
	util.RecyclableMemory
}

func (r *RTPData) GetTimestamp() time.Duration {
	return time.Duration(r.Timestamp) * time.Second / time.Duration(r.ClockRate)
}

func (r *RTPData) GetSize() int {
	return r.Packet.MarshalSize()
}

func (r *RTPData) IsIDR() bool {
	return false
}

type (
	RTPCtx struct {
		webrtc.RTPCodecParameters
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

func (r *RTPData) DecodeConfig(track *AVTrack) error {
	switch r.MimeType {
	case webrtc.MimeTypeOpus:
		track.Codec = codec.FourCC_OPUS
		var ctx RTPOPUSCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypePCMA:
		track.Codec = codec.FourCC_ALAW
		var ctx RTPG711Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypePCMU:
		track.Codec = codec.FourCC_ULAW
		var ctx RTPG711Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypeH264:
		track.Codec = codec.FourCC_H264
		var ctx RTPH264Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypeVP9:
		track.Codec = codec.FourCC_VP9
		var ctx RTPVP9Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypeAV1:
		track.Codec = codec.FourCC_AV1
		var ctx RTPAV1Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case webrtc.MimeTypeH265:
		track.Codec = codec.FourCC_H265
		var ctx RTPH265Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	case "audio/MPEG4-GENERIC", "audio/AAC":
		track.Codec = codec.FourCC_MP4A
		var ctx RTPAACCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		track.ICodecCtx = &ctx
	default:
		return ErrUnsupportCodec
	}
	return nil
}

type RTPAudio struct {
	RTPData
}

func (r *RTPAudio) FromRaw(*AVTrack, any) error {
	panic("unimplemented")
}

func (r *RTPAudio) ToRaw(*AVTrack) (any, error) {
	return r.Payload, nil
}
