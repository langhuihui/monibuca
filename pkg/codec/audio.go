package codec

import (
	"fmt"
)

type (
	AudioCtx struct {
		SampleRate int
		Channels   int
		SampleSize int
	}
	PCMACtx struct {
		AudioCtx
	}
	PCMUCtx struct {
		AudioCtx
	}
	OPUSCtx struct {
		AudioCtx
	}
	AACCtx struct {
		AudioCtx
		Asc []byte
	}
)

func (ctx *AudioCtx) GetSampleRate() int {
	return ctx.SampleRate
}

func (ctx *AudioCtx) GetChannels() int {
	return ctx.Channels
}

func (ctx *AudioCtx) GetSampleSize() int {
	return ctx.SampleSize
}

func (ctx *AudioCtx) GetInfo() string {
	return fmt.Sprintf("sample rate: %d, channels: %d, sample size: %d", ctx.SampleRate, ctx.Channels, ctx.SampleSize)
}

func (ctx *AACCtx) GetBase() ICodecCtx {
	return ctx
}

func (*PCMUCtx) FourCC() FourCC {
	return FourCC_ULAW
}

func (*PCMACtx) FourCC() FourCC {
	return FourCC_ALAW
}

func (ctx *PCMACtx) GetBase() ICodecCtx {
	return ctx
}

func (ctx *PCMUCtx) GetBase() ICodecCtx {
	return ctx
}

func (*AACCtx) FourCC() FourCC {
	return FourCC_MP4A
}

func (*OPUSCtx) FourCC() FourCC {
	return FourCC_OPUS
}

func (ctx *OPUSCtx) GetBase() ICodecCtx {
	return ctx
}
