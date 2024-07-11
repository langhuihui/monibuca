package m7s

import (
	"m7s.live/m7s/v5/pkg"
)

type Transformer struct {
	*Publisher
	*Subscriber
}

func (t *Transformer) Transform() {
	PlayBlock(t.Subscriber, func(audioFrame *pkg.AVFrame) error {
		//t.Publisher.WriteAudio()
		return nil
	}, func(videoFrame *pkg.AVFrame) error {
		//t.Publisher.WriteVideo()
		return nil
	})
}
