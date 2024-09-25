package sei

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type Transformer struct {
	m7s.DefaultTransformer
	data      chan util.Buffer
	allocator *util.ScalableMemoryAllocator
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
		data:      make(chan util.Buffer, 10),
		allocator: util.NewScalableMemoryAllocator(1 << util.MinPowerOf2),
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
	m7s.PlayBlock(t.TransformJob.Subscriber, func(audio *pkg.RawAudio) (err error) {
		copyAudio := &pkg.RawAudio{
			FourCC:    audio.FourCC,
			Timestamp: audio.Timestamp,
		}
		copyAudio.SetAllocator(t.allocator)
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
		copyVideo.SetAllocator(t.allocator)
		if len(t.data) > 0 {
			for seiFrame := range t.data {
				switch video.FourCC {
				case codec.FourCC_H264:
					var seiNalu util.Memory
					seiNalu.Append([]byte{byte(codec.NALU_SEI)}, seiFrame)
					copyVideo.Nalus = append(copyVideo.Nalus, seiNalu)
				}
				for _, nalu := range video.Nalus {
					mem := copyVideo.NextN(nalu.Size)
					copy(mem, nalu.ToBytes())
					copyVideo.Nalus.Append(mem)
				}
			}
		}
		return t.TransformJob.Publisher.WriteVideo(copyVideo)
	})
	return
}

func (t *Transformer) Dispose() {
	close(t.data)
	t.allocator.Recycle()
}
