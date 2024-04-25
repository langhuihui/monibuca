package rtmp

import (
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
)

type RTMPAudio struct {
	RTMPData
}

func (avcc *RTMPAudio) DecodeConfig(from ICodecCtx) (codecCtx ICodecCtx, err error) {
	if avcc == nil {
		switch fourCC := from.Codec(); fourCC {
		case codec.FourCC_ALAW,codec.FourCC_ULAW:
			var ctx G711Ctx
			ctx.FourCC = fourCC
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			codecCtx = &ctx
		case codec.FourCC_MP4A:
			var ctx AACCtx
			ctx.FourCC = fourCC
			ctx.SampleRate = 44100
			ctx.Channels = 2
			ctx.SampleSize = 16
			codecCtx = &ctx
		}
		return
	}
	reader := avcc.Buffers
	var b byte
	b, err = reader.ReadByte()
	if err != nil {
		return
	}
	b0 := b
	b, err = reader.ReadByte()
	if err != nil {
		return
	}
	b1 := b
	if b1 == 0 {
		switch b0 & 0b1111_0000 >> 4 {
		case 7:
			var ctx G711Ctx
			ctx.FourCC = codec.FourCC_ALAW
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			codecCtx = &ctx
		case 8:
			var ctx G711Ctx
			ctx.FourCC = codec.FourCC_ULAW
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			codecCtx = &ctx
		case 10:
			var ctx AACCtx
			ctx.FourCC = codec.FourCC_MP4A
			b0, err = reader.ReadByte()
			if err != nil {
				return
			}
			b1, err = reader.ReadByte()
			if err != nil {
				return
			}
			ctx.AudioObjectType = b0 >> 3
			ctx.SamplingFrequencyIndex = (b0 & 0x07 << 1) | (b1 >> 7)
			ctx.ChannelConfiguration = (b1 >> 3) & 0x0F
			ctx.FrameLengthFlag = (b1 >> 2) & 0x01
			ctx.DependsOnCoreCoder = (b1 >> 1) & 0x01
			ctx.ExtensionFlag = b1 & 0x01
			ctx.SequenceFrame = avcc
			ctx.Channels = int(ctx.ChannelConfiguration)
			ctx.SampleRate = SamplingFrequencies[ctx.SamplingFrequencyIndex]
			ctx.SampleSize = 16
			codecCtx = &ctx
		}
	}
	return
}

func (avcc *RTMPAudio) ToRaw(codecCtx ICodecCtx) (any, error) {
	reader := avcc.Buffers
	if _, ok := codecCtx.(*AACCtx); ok {
		err := reader.Skip(2)
		return reader.Buffers, err
	} else {
		err := reader.Skip(1)
		return reader.Buffers, err
	}
}

func (g711 *G711Ctx) CreateFrame(raw any) (frame IAVFrame, err error) {
	return
}
