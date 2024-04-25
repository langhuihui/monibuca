package rtmp

import (
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

var _ IAVFrame = (*RTMPVideo)(nil)

type RTMPVideo struct {
	RTMPData
}

func (avcc *RTMPVideo) IsIDR() bool {
	return avcc.Buffers.Buffers[0][0]&0b0111_0000>>4 == 1
}

func (avcc *RTMPVideo) DecodeConfig(from ICodecCtx) (codecCtx ICodecCtx, err error) {
	if avcc == nil {
		switch fourCC := from.Codec(); fourCC {
		case codec.FourCC_H264:
			var ctx H264Ctx
			ctx.FourCC = fourCC
			codecCtx = &ctx
		}
		return
	}
	reader := avcc.Buffers
	var b0 byte
	b0, err = reader.ReadByte()
	if err != nil {
		return
	}
	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111
	var fourCC codec.FourCC
	parseSequence := func() (err error) {
		switch fourCC {
		case codec.FourCC_H264:
			var ctx H264Ctx
			ctx.FourCC = fourCC
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				codecCtx = &ctx
			}
		case codec.FourCC_H265:
			var ctx H265Ctx
			ctx.FourCC = fourCC
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				codecCtx = &ctx
			}
		case codec.FourCC_AV1:
			var ctx AV1Ctx
			ctx.FourCC = fourCC
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				codecCtx = &ctx
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
			if _, err = avcc.DecodeConfig(nil); err != nil {
				return nil, err
			}
			return nil, nil
		case PacketTypeCodedFrames:
			if codecCtx.Is(codec.FourCC_H265) {
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
			if _, err = avcc.DecodeConfig(nil); err != nil {
				return nil, err
			}
		} else {
			if codecCtx.Is(codec.FourCC_H265) {
				return avcc.parseH265(codecCtx.(*H265Ctx), &reader)
			} else {
				return avcc.parseH264(codecCtx.(*H264Ctx), &reader)
			}
		}
	}
	return nil, nil
}

func (h264 *H264Ctx) CreateFrame(raw any) (frame IAVFrame, err error) {
	return
}

func (av1 *AV1Ctx) CreateFrame(raw any) (frame IAVFrame, err error) {
	return
}
