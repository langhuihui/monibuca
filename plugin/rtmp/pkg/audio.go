package rtmp

import (
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
)

type RTMPAudio struct {
	RTMPData
}

func (avcc *RTMPAudio) Parse(t *AVTrack) (isIDR, isSeq bool, raw any, err error) {
	reader := avcc.NewReader()
	var b, b0, b1 byte
	b, err = reader.ReadByte()
	if err != nil {
		return
	}
	switch b & 0b1111_0000 >> 4 {
	case 7:
		if t.ICodecCtx == nil {
			var ctx PCMACtx
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			t.ICodecCtx = &ctx
		}
	case 8:
		if t.ICodecCtx == nil {
			var ctx PCMUCtx
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			t.ICodecCtx = &ctx
		}
	case 10:
		b, err = reader.ReadByte()
		if err != nil {
			return
		}
		isSeq = b == 0
		if isSeq {
			var ctx AACCtx
			b0, err = reader.ReadByte()
			if err != nil {
				return
			}
			b1, err = reader.ReadByte()
			if err != nil {
				return
			}
			var cloneFrame RTMPAudio
			cloneFrame.CopyFrom(avcc.Memory)
			ctx.AudioObjectType = b0 >> 3
			ctx.SamplingFrequencyIndex = (b0 & 0x07 << 1) | (b1 >> 7)
			ctx.ChannelConfiguration = (b1 >> 3) & 0x0F
			ctx.FrameLengthFlag = (b1 >> 2) & 0x01
			ctx.DependsOnCoreCoder = (b1 >> 1) & 0x01
			ctx.ExtensionFlag = b1 & 0x01
			ctx.Channels = int(ctx.ChannelConfiguration)
			ctx.SampleRate = SamplingFrequencies[ctx.SamplingFrequencyIndex]
			ctx.SampleSize = 16
			t.SequenceFrame = &cloneFrame
			t.ICodecCtx = &ctx
		}
	}
	return
}

func (avcc *RTMPAudio) DecodeConfig(t *AVTrack, from ICodecCtx) (err error) {
	switch fourCC := from.FourCC(); fourCC {
	case codec.FourCC_ALAW:
		var ctx PCMACtx
		t.ICodecCtx = &ctx
	case codec.FourCC_ULAW:
		var ctx PCMUCtx
		ctx.SampleRate = 8000
		ctx.Channels = 1
		ctx.SampleSize = 8
		t.ICodecCtx = &ctx
	case codec.FourCC_MP4A:
		var ctx AACCtx
		ctx.SampleRate = 44100
		ctx.Channels = 2
		ctx.SampleSize = 16
		t.ICodecCtx = &ctx
	}
	return
}

func (avcc *RTMPAudio) ToRaw(codecCtx ICodecCtx) (any, error) {
	reader := avcc.NewReader()
	if _, ok := codecCtx.(*AACCtx); ok {
		err := reader.Skip(2)
		return reader.Memory, err
	} else {
		err := reader.Skip(1)
		return reader.Memory, err
	}
}

func (aac *AACCtx) CreateFrame(*AVFrame) (frame IAVFrame, err error) {
	var rtmpAudio RTMPAudio
	frame = &rtmpAudio
	return
}

func (g711 *PCMACtx) CreateFrame(*AVFrame) (frame IAVFrame, err error) {
	var rtmpAudio RTMPAudio
	frame = &rtmpAudio
	return
}

func (g711 *PCMUCtx) CreateFrame(*AVFrame) (frame IAVFrame, err error) {
	var rtmpAudio RTMPAudio
	frame = &rtmpAudio
	return
}
