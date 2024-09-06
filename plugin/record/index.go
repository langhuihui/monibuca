package plugin_record

import (
	"fmt"
	"net/http"

	"m7s.live/m7s/v5"
	record "m7s.live/m7s/v5/plugin/record/pkg"
)

type (
	RecordPlugin struct {
		m7s.Plugin
		Path string `default:"record"`
	}
)

var defaultYaml m7s.DefaultYaml = `subscribe:
  submode: 1
`

var _ = m7s.InstallPlugin[RecordPlugin](defaultYaml, record.NewRecorder)

func (plugin *RecordPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/vod/flv/{streamPath...}": plugin.vodFLV,
		"/vod/mp4/{streamPath...}": plugin.vodMP4,
	}
}

func (plugin *RecordPlugin) OnInit() (err error) {
	if plugin.DB == nil {
		return fmt.Errorf("db not found")
	}
	return
}

func (plugin *RecordPlugin) vodMP4(w http.ResponseWriter, r *http.Request) {

}
