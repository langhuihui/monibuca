package codec

type (
	AudioCtx struct {
		SampleRate int
		Channels   int
		SampleSize int
	}
	PCMACtx AudioCtx
	PCMUCtx AudioCtx
	AACCtx  struct {
		AudioCtx
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

func (*PCMUCtx) FourCC() FourCC {
	return FourCC_ULAW
}

func (*PCMACtx) FourCC() FourCC {
	return FourCC_ALAW
}

func (*AACCtx) FourCC() FourCC {
	return FourCC_MP4A
}
