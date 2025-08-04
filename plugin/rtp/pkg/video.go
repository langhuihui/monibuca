package rtp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"slices"
	"time"
	"unsafe"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
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
		*codec.H264Ctx
	}
	H265Ctx struct {
		H26xCtx
		*codec.H265Ctx
		DONL bool
	}
	AV1Ctx struct {
		RTPCtx
		*codec.AV1Ctx
	}
	VP9Ctx struct {
		RTPCtx
	}
	VideoFrame struct {
		RTPData
	}
)

var (
	_ IAVFrame       = (*VideoFrame)(nil)
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

func (r *VideoFrame) Parse(data IAVFrame) (err error) {
	input := data.(*VideoFrame)
	r.Packets = append(r.Packets[:0], input.Packets...)
	return
}

func (r *VideoFrame) Recycle() {
	r.RecyclableMemory.Recycle()
	r.Packets.Reset()
}

func (r *VideoFrame) CheckCodecChange() (err error) {
	if len(r.Packets) == 0 {
		return ErrSkip
	}
	old := r.ICodecCtx
	// 解复用数据
	if err = r.Demux(); err != nil {
		return
	}
	// 处理时间戳和序列号
	pts := r.Packets[0].Timestamp
	nalus := r.Raw.(*Nalus)
	switch ctx := old.(type) {
	case *H264Ctx:
		dts := ctx.dtsEst.Feed(pts)
		r.SetDTS(time.Duration(dts))
		r.SetPTS(time.Duration(pts))

		// 检查 SPS、PPS 和 IDR 帧
		var sps, pps []byte
		var hasSPSPPS bool
		for nalu := range nalus.RangePoint {
			nalType := codec.ParseH264NALUType(nalu.Buffers[0][0])
			switch nalType {
			case h264parser.NALU_SPS:
				sps = nalu.ToBytes()
				defer nalus.Remove(nalu)
			case h264parser.NALU_PPS:
				pps = nalu.ToBytes()
				defer nalus.Remove(nalu)
			case codec.NALU_IDR_Picture:
				r.IDR = true
			}
		}

		// 如果发现新的 SPS/PPS，更新编解码器上下文
		if hasSPSPPS = sps != nil && pps != nil; hasSPSPPS && (len(ctx.Record) == 0 || !bytes.Equal(sps, ctx.SPS()) || !bytes.Equal(pps, ctx.PPS())) {
			var newCodecData h264parser.CodecData
			if newCodecData, err = h264parser.NewCodecDataFromSPSAndPPS(sps, pps); err != nil {
				return
			}
			newCtx := &H264Ctx{
				H26xCtx: ctx.H26xCtx,
				H264Ctx: &codec.H264Ctx{
					CodecData: newCodecData,
				},
			}
			// 保持原有的 RTP 参数
			if oldCtx, ok := old.(*H264Ctx); ok {
				newCtx.RTPCtx = oldCtx.RTPCtx
			}
			r.ICodecCtx = newCtx
		} else {
			// 如果是 IDR 帧但没有 SPS/PPS，需要插入
			if r.IDR && len(ctx.SPS()) > 0 && len(ctx.PPS()) > 0 {
				spsRTP := rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						SequenceNumber: ctx.SequenceNumber,
						Timestamp:      pts,
						SSRC:           ctx.SSRC,
						PayloadType:    uint8(ctx.PayloadType),
					},
					Payload: ctx.SPS(),
				}
				ppsRTP := rtp.Packet{
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
		}

		// 更新序列号
		for p := range r.Packets.RangePoint {
			p.SequenceNumber = ctx.seq
			ctx.seq++
		}
	case *H265Ctx:
		dts := ctx.dtsEst.Feed(pts)
		r.SetDTS(time.Duration(dts))
		r.SetPTS(time.Duration(pts))
		// 检查 VPS、SPS、PPS 和 IDR 帧
		var vps, sps, pps []byte
		var hasVPSSPSPPS bool
		for nalu := range nalus.RangePoint {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				vps = nalu.ToBytes()
				defer nalus.Remove(nalu)
			case h265parser.NAL_UNIT_SPS:
				sps = nalu.ToBytes()
				defer nalus.Remove(nalu)
			case h265parser.NAL_UNIT_PPS:
				pps = nalu.ToBytes()
				defer nalus.Remove(nalu)
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				r.IDR = true
			}
		}

		// 如果发现新的 VPS/SPS/PPS，更新编解码器上下文
		if hasVPSSPSPPS = vps != nil && sps != nil && pps != nil; hasVPSSPSPPS && (len(ctx.Record) == 0 || !bytes.Equal(vps, ctx.VPS()) || !bytes.Equal(sps, ctx.SPS()) || !bytes.Equal(pps, ctx.PPS())) {
			var newCodecData h265parser.CodecData
			if newCodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps); err != nil {
				return
			}
			newCtx := &H265Ctx{
				H26xCtx: ctx.H26xCtx,
				H265Ctx: &codec.H265Ctx{
					CodecData: newCodecData,
				},
			}
			// 保持原有的 RTP 参数
			if oldCtx, ok := old.(*H265Ctx); ok {
				newCtx.RTPCtx = oldCtx.RTPCtx
			}
			r.ICodecCtx = newCtx
		} else {
			// 如果是 IDR 帧但没有 VPS/SPS/PPS，需要插入
			if r.IDR && len(ctx.VPS()) > 0 && len(ctx.SPS()) > 0 && len(ctx.PPS()) > 0 {
				vpsRTP := rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						SequenceNumber: ctx.SequenceNumber,
						Timestamp:      pts,
						SSRC:           ctx.SSRC,
						PayloadType:    uint8(ctx.PayloadType),
					},
					Payload: ctx.VPS(),
				}
				spsRTP := rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						SequenceNumber: ctx.SequenceNumber,
						Timestamp:      pts,
						SSRC:           ctx.SSRC,
						PayloadType:    uint8(ctx.PayloadType),
					},
					Payload: ctx.SPS(),
				}
				ppsRTP := rtp.Packet{
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
		}

		// 更新序列号
		for p := range r.Packets.RangePoint {
			p.SequenceNumber = ctx.seq
			ctx.seq++
		}
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

func (r *VideoFrame) Mux(baseFrame *Sample) error {
	// 获取编解码器上下文
	codecCtx := r.ICodecCtx
	if codecCtx == nil {
		switch base := baseFrame.GetBase().(type) {
		case *codec.H264Ctx:
			var ctx H264Ctx
			ctx.H264Ctx = base
			ctx.PayloadType = 96
			ctx.MimeType = webrtc.MimeTypeH264
			ctx.ClockRate = 90000
			spsInfo := ctx.SPSInfo
			ctx.SDPFmtpLine = fmt.Sprintf("sprop-parameter-sets=%s,%s;profile-level-id=%02x%02x%02x;level-asymmetry-allowed=1;packetization-mode=1", base64.StdEncoding.EncodeToString(ctx.SPS()), base64.StdEncoding.EncodeToString(ctx.PPS()), spsInfo.ProfileIdc, spsInfo.ConstraintSetFlag, spsInfo.LevelIdc)
			ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
			codecCtx = &ctx
		case *codec.H265Ctx:
			var ctx H265Ctx
			ctx.H265Ctx = base
			ctx.PayloadType = 98
			ctx.MimeType = webrtc.MimeTypeH265
			ctx.SDPFmtpLine = fmt.Sprintf("profile-id=1;sprop-sps=%s;sprop-pps=%s;sprop-vps=%s", base64.StdEncoding.EncodeToString(ctx.SPS()), base64.StdEncoding.EncodeToString(ctx.PPS()), base64.StdEncoding.EncodeToString(ctx.VPS()))
			ctx.ClockRate = 90000
			ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
			codecCtx = &ctx
		}
		r.ICodecCtx = codecCtx
	}
	// 获取时间戳信息
	pts := uint32(baseFrame.GetPTS())

	switch c := codecCtx.(type) {
	case *H264Ctx:
		ctx := &c.RTPCtx
		var lastPacket *rtp.Packet
		if baseFrame.IDR && len(c.RecordInfo.SPS) > 0 && len(c.RecordInfo.PPS) > 0 {
			r.Append(ctx, pts, c.SPS())
			r.Append(ctx, pts, c.PPS())
		}
		for nalu := range baseFrame.Raw.(*Nalus).RangePoint {
			if reader := nalu.NewReader(); reader.Length > MTUSize {
				payloadLen := MTUSize
				if reader.Length+1 < payloadLen {
					payloadLen = reader.Length + 1
				}
				//fu-a
				mem := r.NextN(payloadLen)
				reader.Read(mem[1:])
				fuaHead, naluType := codec.NALU_FUA.Or(mem[1]&0x60), mem[1]&0x1f
				mem[0], mem[1] = fuaHead, naluType|startBit
				lastPacket = r.Append(ctx, pts, mem)
				for payloadLen = MTUSize; reader.Length > 0; lastPacket = r.Append(ctx, pts, mem) {
					if reader.Length+2 < payloadLen {
						payloadLen = reader.Length + 2
					}
					mem = r.NextN(payloadLen)
					reader.Read(mem[2:])
					mem[0], mem[1] = fuaHead, naluType
				}
				lastPacket.Payload[1] |= endBit
			} else {
				mem := r.NextN(reader.Length)
				reader.Read(mem)
				lastPacket = r.Append(ctx, pts, mem)
			}
		}
		lastPacket.Header.Marker = true
	case *H265Ctx:
		ctx := &c.RTPCtx
		var lastPacket *rtp.Packet
		if baseFrame.IDR && len(c.RecordInfo.SPS) > 0 && len(c.RecordInfo.PPS) > 0 && len(c.RecordInfo.VPS) > 0 {
			r.Append(ctx, pts, c.VPS())
			r.Append(ctx, pts, c.SPS())
			r.Append(ctx, pts, c.PPS())
		}
		for nalu := range baseFrame.Raw.(*Nalus).RangePoint {
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
				reader.Read(mem[3:])
				mem[0], mem[1], mem[2] = b0, b1, naluType|startBit
				lastPacket = r.Append(ctx, pts, mem)

				for payloadLen = MTUSize; reader.Length > 0; lastPacket = r.Append(ctx, pts, mem) {
					if reader.Length+3 < payloadLen {
						payloadLen = reader.Length + 3
					}
					mem = r.NextN(payloadLen)
					reader.Read(mem[3:])
					mem[0], mem[1], mem[2] = b0, b1, naluType
				}
				lastPacket.Payload[2] |= endBit
			} else {
				mem := r.NextN(reader.Length)
				reader.Read(mem)
				lastPacket = r.Append(ctx, pts, mem)
			}
		}
		lastPacket.Header.Marker = true
	}
	return nil
}

func (r *VideoFrame) Demux() (err error) {
	if len(r.Packets) == 0 {
		return ErrSkip
	}
	switch c := r.ICodecCtx.(type) {
	case *H264Ctx:
		nalus := r.GetNalus()
		nalu := nalus.GetNextPointer()
		var naluType codec.H264NALUType
		gotNalu := func() {
			if nalu.Size > 0 {
				nalu = nalus.GetNextPointer()
			}
		}
		for packet := range r.Packets.RangePoint {
			if len(packet.Payload) < 2 {
				continue
			}
			if packet.Padding {
				packet.Padding = false
			}
			b0 := packet.Payload[0]
			if t := codec.ParseH264NALUType(b0); t < 24 {
				nalu.PushOne(packet.Payload)
				gotNalu()
			} else {
				offset := t.Offset()
				switch t {
				case codec.NALU_STAPA, codec.NALU_STAPB:
					if len(packet.Payload) <= offset {
						return fmt.Errorf("invalid nalu size %d", len(packet.Payload))
					}
					for buffer := util.Buffer(packet.Payload[offset:]); buffer.CanRead(); {
						if nextSize := int(buffer.ReadUint16()); buffer.Len() >= nextSize {
							nalu.PushOne(buffer.ReadN(nextSize))
							gotNalu()
						} else {
							return fmt.Errorf("invalid nalu size %d", nextSize)
						}
					}
				case codec.NALU_FUA, codec.NALU_FUB:
					b1 := packet.Payload[1]
					if util.Bit1(b1, 0) {
						naluType.Parse(b1)
						nalu.PushOne([]byte{naluType.Or(b0 & 0x60)})
					}
					if nalu.Size > 0 {
						nalu.PushOne(packet.Payload[offset:])
					} else {
						continue
					}
					if util.Bit1(b1, 1) {
						gotNalu()
					}
				default:
					return fmt.Errorf("unsupported nalu type %d", t)
				}
			}
		}
		nalus.Reduce()
		return nil
	case *H265Ctx:
		nalus := r.GetNalus()
		nalu := nalus.GetNextPointer()
		gotNalu := func() {
			if nalu.Size > 0 {
				nalu = nalus.GetNextPointer()
			}
		}
		for _, packet := range r.Packets {
			if len(packet.Payload) == 0 {
				continue
			}
			b0 := packet.Payload[0]
			if t := codec.ParseH265NALUType(b0); t < H265_NALU_AP {
				nalu.PushOne(packet.Payload)
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
						nalu.PushOne(buffer.ReadN(int(buffer.ReadUint16())))
						gotNalu()
					}
					if c.DONL {
						buffer.ReadByte()
					}
				case H265_NALU_FU:
					if buffer.Len() < 3 {
						return io.ErrShortBuffer
					}
					first3 := buffer.ReadN(3)
					fuHeader := first3[2]
					if c.DONL {
						buffer.ReadUint16()
					}
					if naluType := fuHeader & 0b00111111; util.Bit1(fuHeader, 0) {
						nalu.PushOne([]byte{first3[0]&0b10000001 | (naluType << 1), first3[1]})
					}
					nalu.PushOne(buffer)
					if util.Bit1(fuHeader, 1) {
						gotNalu()
					}
				default:
					return fmt.Errorf("unsupported nalu type %d", t)
				}
			}
		}
		nalus.Reduce()
		return nil
	}
	return ErrUnsupportCodec
}
