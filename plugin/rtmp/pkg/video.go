package rtmp

import (
	"context"
	"encoding/binary"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

var _ IAVFrame = (*RTMPVideo)(nil)

type RTMPVideo struct {
	RTMPData
}

func (avcc *RTMPVideo) Parse(t *AVTrack) (isIDR, isSeq bool, raw any, err error) {
	reader := avcc.Buffers
	var b0 byte
	b0, err = reader.ReadByte()
	if err != nil {
		return
	}
	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	isIDR = b0&0b0111_0000>>4 == 1
	packetType := b0 & 0b1111
	var fourCC codec.FourCC
	parseSequence := func() (err error) {
		isSeq = true
		isIDR = false
		switch fourCC {
		case codec.FourCC_H264:
			var ctx H264Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				t.SequenceFrame = avcc
				t.ICodecCtx = &ctx
			}
		case codec.FourCC_H265:
			var ctx H265Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				t.SequenceFrame = avcc
				t.ICodecCtx = &ctx
			}
		case codec.FourCC_AV1:
			var ctx AV1Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				t.SequenceFrame = avcc
				t.ICodecCtx = &ctx
			}
		}
		return
	}
	if enhanced {
		reader.ReadBytesTo(fourCC[:])
		switch packetType {
		case PacketTypeSequenceStart:
			err = parseSequence()
			return
		case PacketTypeCodedFrames:

		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return
		}
		if VideoCodecID(b0&0x0F) == CodecID_H265 {
			fourCC = codec.FourCC_H265
		} else {
			fourCC = codec.FourCC_H264
		}
		_, err = reader.ReadBE(3) // cts == 0
		if err != nil {
			return
		}
		if b0 == 0 {
			if err = parseSequence(); err != nil {
				return
			}
		} else {
			// var naluLen int
			// for reader.Length > 0 {
			// 	naluLen, err = reader.ReadBE(4) // naluLenM
			// 	if err != nil {
			// 		return
			// 	}
			// 	_, n := reader.ReadN(naluLen)
			// 	fmt.Println(avcc.Timestamp, n)
			// 	if n != naluLen {
			// 		err = fmt.Errorf("naluLen:%d != n:%d", naluLen, n)
			// 		return
			// 	}
			// }
		}
	}
	return
}

func (avcc *RTMPVideo) DecodeConfig(t *AVTrack, from ICodecCtx) (err error) {
	switch fourCC := from.FourCC(); fourCC {
	case codec.FourCC_H264:
		h264ctx := from.(codec.IH264Ctx).GetH264Ctx()
		var ctx H264Ctx
		ctx.H264Ctx = *h264ctx
		lenSPS := len(h264ctx.SPS[0])
		lenPPS := len(h264ctx.PPS[0])
		var b util.Buffer
		if lenSPS > 3 {
			b.Write(RTMP_AVC_HEAD[:6])
			b.Write(h264ctx.SPS[0][1:4])
			b.Write(RTMP_AVC_HEAD[9:10])
		} else {
			b.Write(RTMP_AVC_HEAD)
		}
		b.WriteByte(0xE1)
		b.WriteUint16(uint16(lenSPS))
		b.Write(h264ctx.SPS[0])
		b.WriteByte(0x01)
		b.WriteUint16(uint16(lenPPS))
		b.Write(h264ctx.PPS[0])
		t.ICodecCtx = &ctx
		var seqFrame RTMPData
		seqFrame.Buffers.ReadFromBytes(b)
		t.SequenceFrame = seqFrame.WrapVideo()
		if t.Enabled(context.TODO(), TraceLevel) {
			codec := t.FourCC().String()
			size := seqFrame.GetSize()
			data := seqFrame.String()
			t.Trace("decConfig", "codec", codec, "size", size, "data", data)
		}
	}

	return
}

func (avcc *RTMPVideo) parseH264(ctx *H264Ctx, reader *util.Buffers) (any, error) {
	cts, err := reader.ReadBE(3)
	if err != nil {
		return nil, err
	}
	var nalus Nalus
	nalus.PTS = time.Duration(avcc.Timestamp+uint32(cts)) * 90
	nalus.DTS = time.Duration(avcc.Timestamp) * 90
	if err := nalus.ParseAVCC(reader, ctx.NalulenSize); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseH265(ctx *H265Ctx, reader *util.Buffers) (any, error) {
	cts, err := reader.ReadBE(3)
	if err != nil {
		return nil, err
	}
	var nalus Nalus
	nalus.PTS = time.Duration(avcc.Timestamp+uint32(cts)) * 90
	nalus.DTS = time.Duration(avcc.Timestamp) * 90
	if err := nalus.ParseAVCC(reader, ctx.NalulenSize); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseAV1(reader *util.Buffers) (any, error) {
	var obus OBUs
	obus.PTS = time.Duration(avcc.Timestamp) * 90
	if err := obus.ParseAVCC(reader); err != nil {
		return obus, err
	}
	return obus, nil
}

func (avcc *RTMPVideo) ToRaw(codecCtx ICodecCtx) (any, error) {
	reader := avcc.Buffers
	b0, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111

	if enhanced {
		err = reader.Skip(4) // fourcc
		if err != nil {
			return nil, err
		}
		switch packetType {
		case PacketTypeSequenceStart:
			// if _, err = avcc.DecodeConfig(nil); err != nil {
			// 	return nil, err
			// }
			return nil, nil
		case PacketTypeCodedFrames:
			if codecCtx.FourCC() == codec.FourCC_H265 {
				return avcc.parseH265(codecCtx.(*H265Ctx), &reader)
			} else {
				return avcc.parseAV1(&reader)
			}
		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return nil, err
		}
		if b0 == 0 {
			if err = reader.Skip(3); err != nil {
				return nil, err
			}
			// if _, err = avcc.DecodeConfig(nil); err != nil {
			// 	return nil, err
			// }
		} else {
			if codecCtx.FourCC() == codec.FourCC_H265 {
				return avcc.parseH265(codecCtx.(*H265Ctx), &reader)
			} else {
				return avcc.parseH264(codecCtx.(*H264Ctx), &reader)
			}
		}
	}
	return nil, nil
}

func (h264 *H264Ctx) CreateFrame(from *AVFrame) (frame IAVFrame, err error) {
	var rtmpVideo RTMPVideo
	rtmpVideo.Timestamp = uint32(from.Timestamp / time.Millisecond)
	// TODO: rtmpVideo.ScalableMemoryAllocator = from.Wraps[0].GetScalableMemoryAllocator()
	nalus := from.Raw.(Nalus)
	head := rtmpVideo.NextN(5)
	head[0] = util.Conditoinal[byte](from.IDR, 0x10, 0x20) | byte(ParseVideoCodec(h264.FourCC()))
	head[1] = 1
	util.PutBE(head[2:5], (nalus.PTS-nalus.DTS)/90) // cts
	for _, nalu := range nalus.Nalus {
		naluLenM := rtmpVideo.NextN(4)
		naluLen := uint32(util.LenOfBuffers(nalu))
		binary.BigEndian.PutUint32(naluLenM, naluLen)
		rtmpVideo.ReadFromBytes(nalu...)
	}
	frame = &rtmpVideo
	return
}
func (h265 *H265Ctx) CreateFrame(*AVFrame) (frame IAVFrame, err error) {
	return
}
func (av1 *AV1Ctx) CreateFrame(*AVFrame) (frame IAVFrame, err error) {
	return
}
