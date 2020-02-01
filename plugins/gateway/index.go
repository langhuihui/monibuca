package gateway

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os/exec"
	"path"
	"runtime"
	"time"

	. "github.com/langhuihui/monibuca/monica"
)

var (
	config        = new(ListenerConfig)
	sseBegin      = []byte("data: ")
	sseEnd        = []byte("\n\n")
	dashboardPath string
)

type SSE struct {
	http.ResponseWriter
	context.Context
}

func (sse *SSE) Write(data []byte) (n int, err error) {
	if err = sse.Err(); err != nil {
		return
	}
	_, err = sse.ResponseWriter.Write(sseBegin)
	n, err = sse.ResponseWriter.Write(data)
	_, err = sse.ResponseWriter.Write(sseEnd)
	if err != nil {
		return
	}
	sse.ResponseWriter.(http.Flusher).Flush()
	return
}
func NewSSE(w http.ResponseWriter, ctx context.Context) *SSE {
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	header.Set("Access-Control-Allow-Origin", "*")
	return &SSE{
		w,
		ctx,
	}
}

func (sse *SSE) WriteJSON(data interface{}) (err error) {
	var jsonData []byte
	if jsonData, err = json.Marshal(data); err == nil {
		if _, err = sse.Write(jsonData); err != nil {
			return
		}
		return
	}
	return
}
func (sse *SSE) WriteExec(cmd *exec.Cmd) error {
	cmd.Stderr = sse
	cmd.Stdout = sse
	return cmd.Run()
}

func init() {
	_, currentFilePath, _, _ := runtime.Caller(0)
	dashboardPath = path.Join(path.Dir(currentFilePath), "../../dashboard/dist")
	log.Println(dashboardPath)
	InstallPlugin(&PluginConfig{
		Name:   "GateWay",
		Type:   PLUGIN_HOOK,
		Config: config,
		Run:    run,
	})
}
func run() {
	http.HandleFunc("/api/stop", stopPublish)
	http.HandleFunc("/api/summary", summary)
	http.HandleFunc("/api/logs", watchLogs)
	http.HandleFunc("/api/config", getConfig)
	http.HandleFunc("/", website)
	log.Printf("server gateway start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, nil))
}
func getConfig(w http.ResponseWriter, r *http.Request) {
	w.Write(ConfigRaw)
}
func watchLogs(w http.ResponseWriter, r *http.Request) {
	AddWriter(NewSSE(w, r.Context()))
	<-r.Context().Done()
}
func stopPublish(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if streamPath := r.URL.Query().Get("stream"); streamPath != "" {
		if b, ok := AllRoom.Load(streamPath); ok {
			b.(*Room).Cancel()
			w.Write([]byte("success"))
		} else {
			w.Write([]byte("no query stream"))
		}
	} else {
		w.Write([]byte("no such stream"))
	}
}
func website(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Path
	if filePath == "/" {
		filePath = "/index.html"
	} else if filePath == "/docs" {
		filePath = "/docs/index.html"
	}
	if mime := mime.TypeByExtension(path.Ext(filePath)); mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	if f, err := ioutil.ReadFile(dashboardPath + filePath); err == nil {
		if _, err = w.Write(f); err != nil {
			w.WriteHeader(505)
		}
	} else {
		w.Header().Set("Location", "/")
		w.WriteHeader(302)
	}
}
func summary(w http.ResponseWriter, r *http.Request) {
	sse := NewSSE(w, r.Context())
	Summary.Add()
	defer Summary.Done()
	sse.WriteJSON(&Summary)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if sse.WriteJSON(&Summary) != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}
