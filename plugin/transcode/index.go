package plugin_transcode

import (
	"net/http"

	"m7s.live/m7s/v5"
	transcode "m7s.live/m7s/v5/plugin/transcode/pkg"
)

var _ = m7s.InstallPlugin[TranscodePlugin](transcode.NewTransform)

type TranscodePlugin struct {
	m7s.Plugin
}

func (t *TranscodePlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/start": t.api_transcode_start,
	}
}
