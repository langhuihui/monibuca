package m7s

import . "m7s.live/monibuca/v5/pkg"

type Publisher struct {
	Plugin *Plugin
}

func (p *Publisher) WriteVideo(data IVideoData) {
}

func (p *Publisher) WriteAudio(data IAudioData) {
}

func (p *Publisher) WriteData(data IData) {
}
