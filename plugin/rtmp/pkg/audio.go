package rtmp

import (
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	"time"
)

type RTMPAudio struct {
	RTMPData
}

func (avcc *RTMPAudio) Parse(t *AVTrack) (err error) {
	reader := avcc.NewReader()
	var b, b0, b1 byte
	b, err = reader.ReadByte()
	if err != nil {
		return
	}
	switch b & 0b1111_0000 >> 4 {
	case 7:
		if t.ICodecCtx == nil {
			var ctx codec.PCMACtx
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			t.ICodecCtx = &ctx
		}
	case 8:
		if t.ICodecCtx == nil {
			var ctx codec.PCMUCtx
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
		if b == 0 {
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
			cloneFrame.CopyFrom(&avcc.Memory)
			ctx.Asc = []byte{b0, b1}
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

func (avcc *RTMPAudio) ConvertCtx(from codec.ICodecCtx, t *AVTrack) (err error) {
	switch fourCC := from.FourCC(); fourCC {
	case codec.FourCC_MP4A:
		var ctx AACCtx
		ctx.AACCtx = *from.GetBase().(*codec.AACCtx)
		b0, b1 := ctx.Asc[0], ctx.Asc[1]
		ctx.AudioObjectType = b0 >> 3
		ctx.SamplingFrequencyIndex = (b0 & 0x07 << 1) | (b1 >> 7)
		ctx.ChannelConfiguration = (b1 >> 3) & 0x0F
		ctx.FrameLengthFlag = (b1 >> 2) & 0x01
		ctx.DependsOnCoreCoder = (b1 >> 1) & 0x01
		ctx.ExtensionFlag = b1 & 0x01
		t.ICodecCtx = &ctx
	default:
		t.ICodecCtx = from.GetBase()
	}
	return
}

func (avcc *RTMPAudio) Demux(codecCtx codec.ICodecCtx) (raw any, err error) {
	reader := avcc.NewReader()
	var result util.Memory
	if _, ok := codecCtx.(*AACCtx); ok {
		err = reader.Skip(2)
		reader.Range(result.AppendOne)
		return result, err
	} else {
		err = reader.Skip(1)
		reader.Range(result.AppendOne)
		return result, err
	}
}

func (avcc *RTMPAudio) Mux(codecCtx codec.ICodecCtx, from *AVFrame) {
	avcc.Timestamp = uint32(from.Timestamp / time.Millisecond)
	audioData := from.Raw.(AudioData)
	switch c := codecCtx.FourCC(); c {
	case codec.FourCC_MP4A:
		avcc.AppendOne([]byte{0xAF, 0x01})
		avcc.Append(audioData.Buffers...)
	case codec.FourCC_ALAW, codec.FourCC_ULAW:
		avcc.AppendOne([]byte{byte(ParseAudioCodec(c))<<4 | (1 << 1)})
		avcc.Append(audioData.Buffers...)
	}
}
