package rtmp

import (
	"github.com/deepch/vdk/codec/aacparser"
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
	var b byte
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
			var ctx codec.AACCtx
			var cloneFrame RTMPAudio
			cloneFrame.CopyFrom(&avcc.Memory)
			ctx.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(cloneFrame.Buffers[0][2:])
			t.SequenceFrame = &cloneFrame
			t.ICodecCtx = &ctx
		}
	}
	return
}

func (avcc *RTMPAudio) ConvertCtx(from codec.ICodecCtx) (to codec.ICodecCtx, seq IAVFrame, err error) {
	to = from.GetBase()
	switch v := to.(type) {
	case *codec.AACCtx:
		var seqFrame RTMPAudio
		seqFrame.AppendOne(append([]byte{0xAF, 0x00}, v.ConfigBytes...))
		seq = &seqFrame
	}
	return
}

func (avcc *RTMPAudio) Demux(codecCtx codec.ICodecCtx) (raw any, err error) {
	reader := avcc.NewReader()
	var result util.Memory
	if _, ok := codecCtx.(*codec.AACCtx); ok {
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
