package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"mime"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/trace"
	"syscall"

	. "github.com/Monibuca/engine/v3"
	// _ "github.com/Monibuca/plugin-ffmpeg"
	_ "github.com/Monibuca/plugin-gateway/v3"

	_ "github.com/Monibuca/plugin-gb28181/v3"
	_ "github.com/Monibuca/plugin-hdl/v3"
	_ "github.com/Monibuca/plugin-hls/v3"
	_ "github.com/Monibuca/plugin-jessica/v3"
	_ "github.com/Monibuca/plugin-logrotate/v3"
	_ "github.com/Monibuca/plugin-record/v3"
	_ "github.com/Monibuca/plugin-rtmp/v3"
	_ "github.com/Monibuca/plugin-rtsp/v3"
	_ "github.com/Monibuca/plugin-summary"
	_ "github.com/Monibuca/plugin-ts/v3"
	_ "github.com/Monibuca/plugin-webrtc/v3"
)

//go:embed ui/*
var ui embed.FS

func main() {
	addr := flag.String("c", "config.toml", "config file")
	traceProfile := flag.String("traceprofile", "", "write trace profile to file")
	flag.Parse()
	if *traceProfile != "" {
		f, err := os.Create(*traceProfile)
		if err != nil {
			log.Fatal(err)
		}
		trace.Start(f)
		defer f.Close()
		defer trace.Stop()
	}
	if _, err := os.Stat(*addr); err == nil {
		Run(*addr)
	} else {
		Run(filepath.Join(filepath.Dir(os.Args[0]), *addr))
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
