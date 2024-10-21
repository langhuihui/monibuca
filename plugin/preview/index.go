package plugin_preview

import (
	"embed"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"m7s.live/v5"
)

//go:embed ui
var f embed.FS

type PreviewPlugin struct {
	m7s.Plugin
}

var _ = m7s.InstallPlugin[PreviewPlugin]()

func (p *PreviewPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s := "<h1><h1><h2>Live Streams 引擎中正在发布的流</h2>"
		p.Server.CallOnStreamTask(func() error {
			for publisher := range p.Server.Streams.Range {
				s += fmt.Sprintf("<a href='%s'>%s</a> [ %s ]<br>", publisher.StreamPath, publisher.StreamPath, publisher.Plugin.Meta.Name)
			}
			s += "<h2>pull stream on subscribe 订阅时才会触发拉流的流</h2>"
			return nil
		})
		p.Server.Call(func() error {
			for plugin := range p.Server.Plugins.Range {
				if pullPlugin, ok := plugin.GetHandler().(m7s.IPullerPlugin); ok {
					s += fmt.Sprintf("<h3>%s</h3>", plugin.Meta.Name)
					for _, streamPath := range pullPlugin.GetPullableList() {
						s += fmt.Sprintf("<a href='%s'>%s</a><br>", streamPath, streamPath)
					}
				} else if plugin.Meta.Puller != nil {
					s += fmt.Sprintf("<h3>%s</h3>", plugin.Meta.Name)
					for _, streamPath := range slices.Collect(maps.Keys(plugin.GetCommonConf().OnSub.Pull)) {
						s += fmt.Sprintf("<a href='%s'>%s</a><br>", streamPath, streamPath)
					}
				}
			}
			return nil
		})
		w.Write([]byte(s))
		return
	}
	ss := strings.Split(r.URL.Path, "/")
	if b, err := f.ReadFile("ui/" + ss[len(ss)-1]); err == nil {
		w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(ss[len(ss)-1])))
		w.Write(b)
	} else {
		//w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		//w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		b, err = f.ReadFile("ui/demo.html")
		w.Write(b)
	}
}
