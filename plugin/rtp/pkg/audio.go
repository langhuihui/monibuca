package rtp

import (
	"fmt"
	"time"
	"unsafe"

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

func (r *RTPData) String() (s string) {
	for _, p := range r.Packets {
		s += fmt.Sprintf("t: %d, s: %d, p: %d\n", p.Timestamp, p.SequenceNumber, len(p.Payload))
	}
	return
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

type (
	RTPCtx struct {
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
		codec.AACCtx
	}
	IRTPCtx interface {
		GetRTPCodecCapability() webrtc.RTPCodecCapability
	}
)

func (r *RTPCtx) GetInfo() string {
	return r.GetRTPCodecCapability().SDPFmtpLine
}

func (r *RTPCtx) GetRTPCodecCapability() webrtc.RTPCodecCapability {
	return r.RTPCodecCapability
}

func (r *RTPCtx) GetSequenceFrame() IAVFrame {
	return nil
}

func (r *RTPData) DecodeConfig(t *AVTrack, from ICodecCtx) (err error) {
	switch fourCC := from.FourCC(); fourCC {
	case codec.FourCC_H264:
		h264ctx := from.(codec.IH264Ctx).GetH264Ctx()
		var ctx RTPH264Ctx
		ctx.H264Ctx = *h264ctx
		ctx.PayloadType = 96
		ctx.MimeType = webrtc.MimeTypeH264
		ctx.ClockRate = 90000
		spsInfo := h264ctx.SPSInfo
		ctx.SDPFmtpLine = fmt.Sprintf("profile-level-id=%02x%02x%02x;level-asymmetry-allowed=1;packetization-mode=1", spsInfo.ProfileIdc, spsInfo.ConstraintSetFlag, spsInfo.LevelIdc)
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		t.ICodecCtx = &ctx
	case codec.FourCC_H265:
		h265ctx := from.(codec.IH265Ctx).GetH265Ctx()
		var ctx RTPH265Ctx
		ctx.H265Ctx = *h265ctx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		t.ICodecCtx = &ctx
	}
	return
}

type RTPAudio struct {
	RTPData
}

func (r *RTPAudio) Parse(t *AVTrack) (isIDR, isSeq bool, raw any, err error) {
	switch r.MimeType {
	case webrtc.MimeTypeOpus:
		// var ctx RTPOPUSCtx
		// ctx.FourCC = codec.FourCC_OPUS
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypePCMA:
		// var ctx RTPG711Ctx
		// ctx.FourCC = codec.FourCC_ALAW
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypePCMU:
		// var ctx RTPG711Ctx
		// ctx.FourCC = codec.FourCC_ULAW
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	}
	return
}

func (ctx *RTPCtx) CreateFrame(*AVFrame) (IAVFrame, error) {
	panic("unimplemented")
}

func (r *RTPAudio) ToRaw(ICodecCtx) (any, error) {
	return nil, nil
}
