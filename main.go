package main

import (
	"context"
	"embed"
	"flag"
	"mime"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	. "github.com/Monibuca/engine/v3"
	// _ "github.com/Monibuca/plugin-ffmpeg"
	_ "github.com/Monibuca/plugin-gateway/v3"

	_ "gitee.com/m7s/plugin-summary"
	_ "github.com/Monibuca/plugin-gb28181/v3"
	_ "github.com/Monibuca/plugin-hdl/v3"
	_ "github.com/Monibuca/plugin-hls/v3"
	_ "github.com/Monibuca/plugin-jessica/v3"
	_ "github.com/Monibuca/plugin-logrotate/v3"
	_ "github.com/Monibuca/plugin-record/v3"
	_ "github.com/Monibuca/plugin-rtmp/v3"
	_ "github.com/Monibuca/plugin-rtsp/v3"
	_ "github.com/Monibuca/plugin-ts/v3"
	_ "github.com/Monibuca/plugin-webrtc/v3"
)

//go:embed ui/*
var ui embed.FS

func main() {
	addr := flag.String("c", "", "config file")
	flag.Parse()
	if *addr == "" {
		// _, currentFile, _, _ := runtime.Caller(0)
		// configFile := filepath.Join(filepath.Dir(currentFile), "config.toml")
		Run(filepath.Join(filepath.Dir(os.Args[0]), "config.toml"))
	} else {
		Run(*addr)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		filePath := r.URL.Path
		if filePath == "/" {
			filePath = "/index.html"
		}
		if mime := mime.TypeByExtension(path.Ext(filePath)); mime != "" {
			w.Header().Set("Content-Type", mime)
		}
		if f, err := ui.ReadFile("ui" + filePath); err == nil {
			if _, err = w.Write(f); err != nil {
				w.WriteHeader(505)
			}
		} else {
			w.Header().Set("Location", "/")
			w.WriteHeader(302)
		}
	})
	waiter(context.Background())
}

func waiter(ctx context.Context) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)
	<-sigc
	ctx.Done()
}
