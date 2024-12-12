package rtp

import (
	"encoding/base64"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type (
	H26xCtx struct {
		RTPCtx
		seq    uint16
		dtsEst util.DTSEstimator
	}
	H264Ctx struct {
		H26xCtx
		codec.H264Ctx
	}
	H265Ctx struct {
		H26xCtx
		codec.H265Ctx
		DONL bool
	}
	AV1Ctx struct {
		RTPCtx
		codec.AV1Ctx
	}
	VP9Ctx struct {
		RTPCtx
	}
	Video struct {
		RTPData
		CTS time.Duration
		DTS time.Duration
	}
)

var (
	_ IAVFrame       = (*Video)(nil)
	_ IVideoCodecCtx = (*H264Ctx)(nil)
	_ IVideoCodecCtx = (*H265Ctx)(nil)
	_ IVideoCodecCtx = (*AV1Ctx)(nil)
)

const (
	H265_NALU_AP = h265parser.NAL_UNIT_UNSPECIFIED_48
	H265_NALU_FU = h265parser.NAL_UNIT_UNSPECIFIED_49
	startBit     = 1 << 7
	endBit       = 1 << 6
	MTUSize      = 1460
)

func (r *Video) Parse(t *AVTrack) (err error) {
	switch r.MimeType {
	case webrtc.MimeTypeH264:
		var ctx *H264Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*H264Ctx)
		} else {
			ctx = &H264Ctx{}
			ctx.parseFmtpLine(r.RTPCodecParameters)
			var sps, pps []byte
			//packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029
			if sprop, ok := ctx.Fmtp["sprop-parameter-sets"]; ok {
				if sprops := strings.Split(sprop, ","); len(sprops) == 2 {
					if sps, err = base64.StdEncoding.DecodeString(sprops[0]); err != nil {
						return
					}
					if pps, err = base64.StdEncoding.DecodeString(sprops[1]); err != nil {
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
		pts := r.Packets[0].Timestamp
		var hasSPSPPS bool
		dts := ctx.dtsEst.Feed(pts)
		r.DTS = time.Duration(dts) * time.Millisecond / 90
		r.CTS = time.Duration(pts-dts) * time.Millisecond / 90
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case h264parser.NALU_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h264parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h264parser.NALU_PPS:
				hasSPSPPS = true
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				if ctx.CodecData, err = h264parser.NewCodecDataFromSPSAndPPS(ctx.RecordInfo.SPS[0], ctx.RecordInfo.PPS[0]); err != nil {
					return
				}
			case codec.NALU_IDR_Picture:
				t.Value.IDR = true
			}
		}
		if t.Value.IDR && !hasSPSPPS {
			spsRTP := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					SequenceNumber: ctx.SequenceNumber,
					Timestamp:      pts,
					SSRC:           ctx.SSRC,
					PayloadType:    uint8(ctx.PayloadType),
				},
				Payload: ctx.SPS(),
			}
			ppsRTP := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					SequenceNumber: ctx.SequenceNumber,
					Timestamp:      pts,
					SSRC:           ctx.SSRC,
					PayloadType:    uint8(ctx.PayloadType),
				},
				Payload: ctx.PPS(),
			}
			r.Packets = slices.Insert(r.Packets, 0, spsRTP, ppsRTP)
		}
		for _, p := range r.Packets {
			p.SequenceNumber = ctx.seq
			ctx.seq++
		}
	case webrtc.MimeTypeH265:
		var ctx *H265Ctx
		if t.ICodecCtx != nil {
			ctx = t.ICodecCtx.(*H265Ctx)
		} else {
			ctx = &H265Ctx{}
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
			if len(vps) > 0 && len(sps) > 0 && len(pps) > 0 {
				if ctx.CodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps); err != nil {
					return
				}
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
		pts := r.Packets[0].Timestamp
		dts := ctx.dtsEst.Feed(pts)
		r.DTS = time.Duration(dts) * time.Millisecond / 90
		r.CTS = time.Duration(pts-dts) * time.Millisecond / 90
		var hasVPSSPSPPS bool
		for _, nalu := range t.Value.Raw.(Nalus) {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				ctx = &H265Ctx{}
				ctx.RecordInfo.VPS = [][]byte{nalu.ToBytes()}
				ctx.RTPCodecParameters = *r.RTPCodecParameters
				t.ICodecCtx = ctx
			case h265parser.NAL_UNIT_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h265parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h265parser.NAL_UNIT_PPS:
				hasVPSSPSPPS = true
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				if ctx.CodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(ctx.RecordInfo.VPS[0], ctx.RecordInfo.SPS[0], ctx.RecordInfo.PPS[0]); err != nil {
					return
				}
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				t.Value.IDR = true
			}
		}
		if t.Value.IDR && !hasVPSSPSPPS {
			vpsRTP := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					SequenceNumber: ctx.SequenceNumber,
					Timestamp:      pts,
					SSRC:           ctx.SSRC,
					PayloadType:    uint8(ctx.PayloadType),
				},
				Payload: ctx.VPS(),
			}
			spsRTP := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					SequenceNumber: ctx.SequenceNumber,
					Timestamp:      pts,
					SSRC:           ctx.SSRC,
					PayloadType:    uint8(ctx.PayloadType),
				},
				Payload: ctx.SPS(),
			}
			ppsRTP := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					SequenceNumber: ctx.SequenceNumber,
					Timestamp:      pts,
					SSRC:           ctx.SSRC,
					PayloadType:    uint8(ctx.PayloadType),
				},
				Payload: ctx.PPS(),
			}
			r.Packets = slices.Insert(r.Packets, 0, vpsRTP, spsRTP, ppsRTP)
		}
		for _, p := range r.Packets {
			p.SequenceNumber = ctx.seq
			ctx.seq++
		}
	case webrtc.MimeTypeVP9:
		// var ctx RTPVP9Ctx
		// ctx.RTPCodecParameters = *r.RTPCodecParameters
		// codecCtx = &ctx
	case webrtc.MimeTypeAV1:
		var ctx AV1Ctx
		ctx.RTPCodecParameters = *r.RTPCodecParameters
		t.ICodecCtx = &ctx
	default:
		err = ErrUnsupportCodec
	}
	return
}

func (h264 *H264Ctx) GetInfo() string {
	return h264.SDPFmtpLine
}

func (h265 *H265Ctx) GetInfo() string {
	return h265.SDPFmtpLine
}

func (av1 *AV1Ctx) GetInfo() string {
	return av1.SDPFmtpLine
}

func (r *Video) GetTimestamp() time.Duration {
	return r.DTS
}

func (r *Video) GetCTS() time.Duration {
	return r.CTS
}

func (r *Video) Mux(codecCtx codec.ICodecCtx, from *AVFrame) {
	pts := uint32((from.Timestamp + from.CTS) * 90 / time.Millisecond)
	switch c := codecCtx.(type) {
	case *H264Ctx:
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
	case *H265Ctx:
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

func (r *Video) Demux(ictx codec.ICodecCtx) (any, error) {
	switch c := ictx.(type) {
	case *H264Ctx:
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
			if packet.Padding {
				packet.Padding = false
			}
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
						continue
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
	case *H265Ctx:
		var nalus Nalus
		var nalu util.Memory
		gotNalu := func() {
			if nalu.Size > 0 {
				nalus = append(nalus, nalu)
				nalu = util.Memory{}
			}
		}
		for _, packet := range r.Packets {
			if len(packet.Payload) == 0 {
				continue
			}
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
