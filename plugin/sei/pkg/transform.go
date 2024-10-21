package sei

import (
	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
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
	return m7s.PlayBlock(t.TransformJob.Subscriber, func(audio *pkg.RawAudio) (err error) {
		copyAudio := &pkg.RawAudio{
			FourCC:    audio.FourCC,
			Timestamp: audio.Timestamp,
		}
		audio.Memory.Range(func(b []byte) {
			copy(copyAudio.NextN(len(b)), b)
		})
		return t.TransformJob.Publisher.WriteAudio(copyAudio)
	}, func(video *pkg.H26xFrame) (err error) {
		copyVideo := &pkg.H26xFrame{
			FourCC:    video.FourCC,
			CTS:       video.CTS,
			Timestamp: video.Timestamp,
		}

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
		for _, nalu := range video.Nalus {
			mem := copyVideo.NextN(nalu.Size)
			copy(mem, nalu.ToBytes())
			if seiCount > 0 {
				switch video.FourCC {
				case codec.FourCC_H264:
					switch codec.ParseH264NALUType(mem[0]) {
					case codec.NALU_IDR_Picture, codec.NALU_Non_IDR_Picture:
						for _, sei := range seis {
							copyVideo.Nalus.Append(append([]byte{byte(codec.NALU_SEI)}, sei...))
						}
					}
				case codec.FourCC_H265:
					if naluType := codec.ParseH265NALUType(mem[0]); naluType < 21 {
						for _, sei := range seis {
							copyVideo.Nalus.Append(append([]byte{byte(0b10000000 | byte(h265parser.NAL_UNIT_PREFIX_SEI<<1))}, sei...))
						}
					}
				}
			}
			copyVideo.Nalus.Append(mem)
		}
		if seiCount > 0 {
			t.Info("insert sei", "count", seiCount)
		}
		return t.TransformJob.Publisher.WriteVideo(copyVideo)
	})
}

func (t *Transformer) Dispose() {
	close(t.data)
}
