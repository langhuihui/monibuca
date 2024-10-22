package plugin_admin

import (
	"embed"
	"net/http"
	"os"
	"time"

	m7s "m7s.live/pro"
)

type AdminPlugin struct {
	m7s.Plugin
}

var _ = m7s.InstallPlugin[AdminPlugin]()

//go:embed web/*
var uiFiles embed.FS
var fileServer = http.FileServer(http.FS(uiFiles))

func (cfg *AdminPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	embedPath := "/web" + r.URL.Path
	if r.URL.Path == "/" {
		r.URL.Path = "/web/index.html"
	} else {
		r.URL.Path = "/web" + r.URL.Path
	}
	file, err := os.Open("./" + r.URL.Path)
	if err == nil {
		defer file.Close()
		http.ServeContent(w, r, r.URL.Path, time.Now(), file)
		return
	}
	r.URL.Path = embedPath
	fileServer.ServeHTTP(w, r)
}
