package sei

import (
	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	"m7s.live/v5/pkg/util"
)

type Transformer struct {
	m7s.DefaultTransformer
	data chan util.Buffer
}

func (t *Transformer) AddSEI(tp byte, data []byte) {
	l := len(data)
	var buffer util.Buffer
	buffer.WriteByte(tp)
	for l >= 255 {
		buffer.WriteByte(255)
		l -= 255
	}
	buffer.WriteByte(byte(l))
	buffer.Write(data)
	buffer.WriteByte(0x80)
	if len(t.data) == cap(t.data) {
		<-t.data
	}
	t.data <- buffer
}

func NewTransform() m7s.ITransformer {
	ret := &Transformer{
		data: make(chan util.Buffer, 10),
	}
	return ret
}

func (t *Transformer) Start() (err error) {
	return t.TransformJob.Subscribe()
}

func (t *Transformer) Run() (err error) {
	err = t.TransformJob.Publish(t.TransformJob.Config.Output[0].StreamPath)
	if err != nil {
		return
	}
	pub := t.TransformJob.Publisher
	allocator := util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	defer allocator.Recycle()
	writer := m7s.NewPublisherWriter[*format.RawAudio, *format.H26xFrame](pub, allocator)
	return m7s.PlayBlock(t.TransformJob.Subscriber, func(audio *format.RawAudio) (err error) {
		writer.AudioFrame.ICodecCtx = audio.ICodecCtx
		*writer.AudioFrame.BaseSample = *audio.BaseSample
		audio.CopyTo(writer.AudioFrame.NextN(audio.Size))
		err = writer.NextAudio()
		return
	}, func(video *format.H26xFrame) (err error) {
		writer.VideoFrame.ICodecCtx = video.ICodecCtx
		*writer.VideoFrame.BaseSample = *video.BaseSample
		nalus := writer.VideoFrame.GetNalus()
		var seis [][]byte
		continueLoop := true
		for continueLoop {
			select {
			case seiFrame := <-t.data:
				seis = append(seis, seiFrame)
			default:
				continueLoop = false
			}
		}
		seiCount := len(seis)
		writer.VideoFrame.InitRecycleIndexes(video.Raw.Count())
		for nalu := range video.Raw.(*pkg.Nalus).RangePoint {
			p := nalus.GetNextPointer()
			mem := writer.VideoFrame.NextN(nalu.Size)
			nalu.CopyTo(mem)
			if seiCount > 0 {
				switch video.ICodecCtx.FourCC() {
				case codec.FourCC_H264:
					switch codec.ParseH264NALUType(mem[0]) {
					case codec.NALU_IDR_Picture, codec.NALU_Non_IDR_Picture:
						for _, sei := range seis {
							p.Push(append([]byte{byte(codec.NALU_SEI)}, sei...))
						}
					}
				case codec.FourCC_H265:
					if naluType := codec.ParseH265NALUType(mem[0]); naluType < 21 {
						for _, sei := range seis {
							p.Push(append([]byte{byte(0b10000000 | byte(h265parser.NAL_UNIT_PREFIX_SEI<<1))}, sei...))
						}
					}
				}
			}
			p.PushOne(mem)
		}
		if seiCount > 0 {
			t.Info("insert sei", "count", seiCount)
		}
		err = writer.NextVideo()
		return
	})
}

func (t *Transformer) Dispose() {
	close(t.data)
}
