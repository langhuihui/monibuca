package rtp

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
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

func (r *RTPData) Dump(t byte, w io.Writer) {
	m := r.Borrow(3 + len(r.Packets)*2 + r.GetSize())
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
	RTPPCMACtx struct {
		RTPCtx
		codec.PCMACtx
	}
	RTPPCMUCtx struct {
		RTPCtx
		codec.PCMUCtx
	}
	RTPOPUSCtx struct {
		RTPCtx
		codec.OPUSCtx
	}
	RTPAACCtx struct {
		RTPCtx
		codec.AACCtx
	}
	IRTPCtx interface {
		GetRTPCodecParameter() webrtc.RTPCodecParameters
	}
)

func (r *RTPCtx) GetInfo() string {
	return r.GetRTPCodecParameter().SDPFmtpLine
}

func (r *RTPCtx) GetRTPCodecParameter() webrtc.RTPCodecParameters {
	return r.RTPCodecParameters
}

func (r *RTPCtx) GetSequenceFrame() IAVFrame {
	return nil
}

func (r *RTPData) DecodeConfig(t *AVTrack, from ICodecCtx) (err error) {
	switch c := from.(type) {
	case codec.IH264Ctx:
		var ctx RTPH264Ctx
		ctx.H264Ctx = *c.GetH264Ctx()
		ctx.PayloadType = 96
		ctx.MimeType = webrtc.MimeTypeH264
		ctx.ClockRate = 90000
		spsInfo := ctx.SPSInfo
		ctx.SDPFmtpLine = fmt.Sprintf("sprop-parameter-sets=%s,%s;profile-level-id=%02x%02x%02x;level-asymmetry-allowed=1;packetization-mode=1", base64.StdEncoding.EncodeToString(ctx.SPS[0]), base64.StdEncoding.EncodeToString(ctx.PPS[0]), spsInfo.ProfileIdc, spsInfo.ConstraintSetFlag, spsInfo.LevelIdc)
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		t.ICodecCtx = &ctx
	case codec.IH265Ctx:
		var ctx RTPH265Ctx
		ctx.H265Ctx = *c.GetH265Ctx()
		ctx.PayloadType = 98
		ctx.MimeType = webrtc.MimeTypeH265
		ctx.SDPFmtpLine = fmt.Sprintf("profile-id=1;sprop-sps=%s;sprop-pps=%s;sprop-vps=%s", base64.StdEncoding.EncodeToString(ctx.SPS[0]), base64.StdEncoding.EncodeToString(ctx.PPS[0]), base64.StdEncoding.EncodeToString(ctx.VPS[0]))
		ctx.ClockRate = 90000
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		t.ICodecCtx = &ctx
	case codec.IAACCtx:
		var ctx RTPAACCtx
		ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
		ctx.AACCtx = *c.GetAACCtx()
		ctx.MimeType = "audio/MPEG4-GENERIC"
		ctx.SDPFmtpLine = fmt.Sprintf("profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3; config=%s", hex.EncodeToString(ctx.AACCtx.Asc))
		ctx.PayloadType = 97
		ctx.ClockRate = uint32(ctx.SampleRate)
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
		var ctx RTPOPUSCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
	case webrtc.MimeTypePCMA:
		var ctx RTPPCMACtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
	case webrtc.MimeTypePCMU:
		var ctx RTPPCMUCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
	case "audio/MPEG4-GENERIC":
		var ctx RTPAACCtx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
	}
	return
}

func (ctx *RTPCtx) CreateFrame(*AVFrame) (IAVFrame, error) {
	panic("unimplemented")
}

func (r *RTPAudio) ToRaw(ICodecCtx) (any, error) {
	return nil, nil
}
