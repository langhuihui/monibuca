package m7s

import (
	"log/slog"
	"net/url"
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
)

type PubSubBase struct {
	*slog.Logger `json:"-" yaml:"-"`
	Plugin       *Plugin
	StartTime    time.Time
	StreamPath   string
	Args         url.Values
}

func (ps *PubSubBase) Init(p *Plugin, streamPath string) {
	ps.Plugin = p
	if u, err := url.Parse(streamPath); err == nil {
		ps.StreamPath, ps.Args = u.Path, u.Query()
	}
	ps.Logger = p.With("streamPath", ps.StreamPath)
	ps.StartTime = time.Now()
}

type Subscriber struct {
	PubSubBase
	config.Subscribe
	VideoTrackReader *AVRingReader
	AudioTrackReader *AVRingReader
}
