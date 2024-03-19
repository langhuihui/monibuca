package m7s

import (
	"log/slog"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
)

type Publisher struct {
	Config config.Publish
	Plugin *Plugin
	Logger *slog.Logger
}

func (p *Publisher) WriteVideo(data IVideoData) {
}

func (p *Publisher) WriteAudio(data IAudioData) {
}

func (p *Publisher) WriteData(data IData) {
}
