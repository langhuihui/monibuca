package codec

import (
	"fmt"

	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/opusparser"
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
		opusparser.CodecData
	}
	AACCtx struct {
		aacparser.CodecData
	}
)

func NewAACCtxFromRecord(record []byte) (ret *AACCtx, err error) {
	ret = &AACCtx{}
	ret.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(record)
	return
}

func NewPCMACtx() *PCMACtx {
	return &PCMACtx{
		AudioCtx: AudioCtx{
			SampleRate: 90000,
			Channels:   1,
			SampleSize: 16,
		},
	}
}

func NewPCMUCtx() *PCMUCtx {
	return &PCMUCtx{
		AudioCtx: AudioCtx{
			SampleRate: 90000,
			Channels:   1,
			SampleSize: 16,
		},
	}
}

func (ctx *AudioCtx) GetRecord() []byte {
	return []byte{}
}

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

func (ctx *AACCtx) GetChannels() int {
	return ctx.ChannelLayout().Count()
}
func (ctx *AACCtx) GetSampleSize() int {
	return 16
}
func (ctx *AACCtx) GetSampleRate() int {
	return ctx.SampleRate()
}
func (ctx *AACCtx) GetBase() ICodecCtx {
	return ctx
}

func (ctx *AACCtx) String() string {
	// https://www.w3.org/TR/webcodecs-aac-codec-registration/
	return fmt.Sprintf("mp4a.40.%d", ctx.Config.ObjectType)
}

func (ctx *AACCtx) GetRecord() []byte {
	return ctx.ConfigBytes
}
func (ctx *AACCtx) GetInfo() string {
	return fmt.Sprintf("sample rate: %d, channels: %d, object type: %d", ctx.SampleRate(), ctx.GetChannels(), ctx.Config.ObjectType)
}
func (*PCMUCtx) FourCC() FourCC {
	return FourCC_ULAW
}

func (*PCMACtx) FourCC() FourCC {
	return FourCC_ALAW
}
func (*PCMACtx) GetRecord() []byte {
	return []byte{} //TODO
}
func (ctx *PCMACtx) GetBase() ICodecCtx {
	return ctx
}

func (ctx *PCMACtx) String() string {
	return "alaw"
}

func (ctx *PCMUCtx) String() string {
	return "ulaw"
}

func (ctx *PCMUCtx) GetBase() ICodecCtx {
	return ctx
}

func (*PCMUCtx) GetRecord() []byte {
	return []byte{} //TODO
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

func (ctx *OPUSCtx) String() string {
	return "opus"
}

func (ctx *OPUSCtx) GetChannels() int {
	return ctx.ChannelLayout().Count()
}
func (ctx *OPUSCtx) GetSampleSize() int {
	return 16
}
func (ctx *OPUSCtx) GetSampleRate() int {
	return ctx.SampleRate()
}
func (ctx *OPUSCtx) GetInfo() string {
	return fmt.Sprintf("sample rate: %d, channels: %d", ctx.SampleRate(), ctx.ChannelLayout().Count())
}

func (ctx *OPUSCtx) GetRecord() []byte {
	// TODO: 需要实现
	return FourCC_OPUS[:]
}
