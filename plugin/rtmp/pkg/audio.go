package rtmp

import (
	"github.com/deepch/vdk/codec/aacparser"
	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type AudioFrame RTMPData

func (avcc *AudioFrame) CheckCodecChange() (err error) {
	old := avcc.ICodecCtx
	reader := avcc.NewReader()
	var b byte
	b, err = reader.ReadByte()
	if err != nil {
		return
	}
	switch b & 0b1111_0000 >> 4 {
	case 7:
		if old == nil {
			var pcma codec.PCMACtx
			pcma.SampleRate = 8000
			pcma.Channels = 1
			pcma.SampleSize = 8
			avcc.ICodecCtx = &pcma
		} else {
			avcc.ICodecCtx = old
		}
	case 8:
		if old == nil {
			var ctx codec.PCMUCtx
			ctx.SampleRate = 8000
			ctx.Channels = 1
			ctx.SampleSize = 8
			avcc.ICodecCtx = &ctx
		} else {
			avcc.ICodecCtx = old
		}
	case 10:
		b, err = reader.ReadByte()
		if err != nil {
			return
		}
		if b == 0 {
			if old == nil || !avcc.Memory.Equal(&old.(*AACCtx).SequenceFrame.Memory) {
				var c AACCtx
				c.AACCtx = &codec.AACCtx{}
				c.SequenceFrame.CopyFrom(&avcc.Memory)
				c.SequenceFrame.BaseSample = &BaseSample{}
				c.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(c.SequenceFrame.Buffers[0][2:])
				avcc.ICodecCtx = &c
			} else {
				avcc.ICodecCtx = old
				err = ErrSkip
			}
		} else {
			avcc.ICodecCtx = old
		}
	}
	return
}

func (avcc *AudioFrame) Demux() (err error) {
	reader := avcc.NewReader()
	result := avcc.GetAudioData()
	if err = reader.Skip(util.Conditional(avcc.FourCC().Is(codec.FourCC_MP4A), 2, 1)); err == nil {
		reader.Range(result.PushOne)
	}
	return
}

func (avcc *AudioFrame) Mux(fromBase *Sample) (err error) {
	audioData := fromBase.Raw.(*AudioData)
	avcc.InitRecycleIndexes(1)
	switch c := fromBase.GetBase().(type) {
	case *codec.AACCtx:
		if avcc.ICodecCtx == nil {
			ctx := &AACCtx{
				AACCtx: c,
			}
			ctx.SequenceFrame.PushOne(append([]byte{0xAF, 0x00}, c.ConfigBytes...))
			ctx.SequenceFrame.BaseSample = &BaseSample{}
			avcc.ICodecCtx = ctx
		}
		head := avcc.NextN(2)
		head[0], head[1] = 0xAF, 0x01
		avcc.Push(audioData.Buffers...)
	default:
		if avcc.ICodecCtx == nil {
			avcc.ICodecCtx = c
		}
		head := avcc.NextN(1)
		head[0] = byte(ParseAudioCodec(c.FourCC()))<<4 | (1 << 1)
		avcc.Push(audioData.Buffers...)
	}
	return
}
