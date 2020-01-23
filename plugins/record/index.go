package record

import (
	. "github.com/langhuihui/monibuca/monica"
	"net/http"
	"sync"
)

var config = struct {
	Path string
}{}
var recordings = sync.Map{}

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "RecordFlv",
		Type:   PLUGIN_SUBSCRIBER,
		Config: &config,
		Run:    run,
	})
}
func run() {
	http.HandleFunc("/api/record/flv", func(writer http.ResponseWriter, r *http.Request) {
		if streamPath := r.URL.Query().Get("streamPath"); streamPath != "" {
			if err := SaveFlv(streamPath, r.URL.Query().Get("append") != ""); err != nil {
				writer.Write([]byte(err.Error()))
			} else {
				writer.Write([]byte("success"))
			}
		} else {
			writer.Write([]byte("no streamPath"))
		}
	})
	http.HandleFunc("/api/record/flv/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if streamPath := r.URL.Query().Get("streamPath"); streamPath != "" {
			filePath := config.Path + streamPath + ".flv"
			if stream, ok := recordings.Load(filePath); ok {
				output := stream.(*OutputStream)
				output.Close()
				w.Write([]byte("success"))
			} else {
				w.Write([]byte("no query stream"))
			}
		} else {
			w.Write([]byte("no such stream"))
		}
	})
}
