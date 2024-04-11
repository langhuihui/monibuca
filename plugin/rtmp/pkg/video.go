package rtmp

import (
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type RTMPVideo struct {
	RTMPData
}

func (avcc *RTMPVideo) IsIDR() bool {
	return avcc.Buffers.Buffers[0][0]&0b0111_0000>>4 == 1
}

func (avcc *RTMPVideo) DecodeConfig(track *AVTrack) error {
	reader := avcc.Buffers
	b0, err := reader.ReadByte()
	if err != nil {
		return err
	}
	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111

	parseSequence := func() (err error) {
		switch track.Codec {
		case codec.FourCC_H264:
			var ctx H264Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				track.ICodecCtx = &ctx
			}
		case codec.FourCC_H265:
			var ctx H265Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				track.ICodecCtx = &ctx
			}
		case codec.FourCC_AV1:
			var ctx AV1Ctx
			if err = ctx.Unmarshal(&reader); err == nil {
				ctx.SequenceFrame = avcc
				track.ICodecCtx = &ctx
			}
		}
		return
	}
	if enhanced {
		err = reader.ReadBytesTo(track.Codec[:])
		if err != nil {
			return err
		}
		switch packetType {
		case PacketTypeSequenceStart:
			if err = parseSequence(); err != nil {
				return err
			}
			return nil
		case PacketTypeCodedFrames:

		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return err
		}
		if VideoCodecID(b0&0x0F) == CodecID_H265 {
			track.Codec = codec.FourCC_H265
		} else {
			track.Codec = codec.FourCC_H264
		}
		_, err = reader.ReadBE(3) // cts == 0
		if err != nil {
			return err
		}
		if b0 == 0 {
			if err = parseSequence(); err != nil {
				return err
			}
		}
	}
	return nil
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

func (avcc *RTMPVideo) ToRaw(track *AVTrack) (any, error) {
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
			if err = avcc.DecodeConfig(track); err != nil {
				return nil, err
			}
			return nil, nil
		case PacketTypeCodedFrames:
			if track.Codec == codec.FourCC_H265 {
				return avcc.parseH265(track.ICodecCtx.(*H265Ctx), &reader)
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
			if err = avcc.DecodeConfig(track); err != nil {
				return nil, err
			}
		} else {
			if track.Codec == codec.FourCC_H265 {
				return avcc.parseH265(track.ICodecCtx.(*H265Ctx), &reader)
			} else {
				return avcc.parseH264(track.ICodecCtx.(*H264Ctx), &reader)
			}
		}
	}
	return nil, nil
}

func (avcc *RTMPVideo) FromRaw(track *AVTrack, raw any) error {
	return nil
}
