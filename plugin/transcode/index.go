package plugin_transcode

import (
	"m7s.live/m7s/v5"
	transcode "m7s.live/m7s/v5/plugin/transcode/pkg"
)

var (
	//_ m7s.IListenPublishPlugin = (*TranscodePlugin)(nil)
	_ = m7s.InstallPlugin[TranscodePlugin](transcode.NewTransform)
)

type TranscodePlugin struct {
	m7s.Plugin
}
