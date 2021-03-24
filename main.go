package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	. "github.com/Monibuca/engine/v2"
	_ "github.com/Monibuca/plugin-cluster"
	_ "github.com/Monibuca/plugin-gateway"
	_ "github.com/Monibuca/plugin-gb28181"
	_ "github.com/Monibuca/plugin-hdl"
	_ "github.com/Monibuca/plugin-hls"
	_ "github.com/Monibuca/plugin-jessica"
	_ "github.com/Monibuca/plugin-logrotate"
	_ "github.com/Monibuca/plugin-record"
	_ "github.com/Monibuca/plugin-rtmp"
	_ "github.com/Monibuca/plugin-rtsp"
	_ "github.com/Monibuca/plugin-ts"
	_ "github.com/Monibuca/plugin-webrtc"
)

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
	waiter(context.Background())
}

func waiter(ctx context.Context) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)
	<-sigc
	ctx.Done()
}
