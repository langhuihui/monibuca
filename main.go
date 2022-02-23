package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	. "github.com/Monibuca/engine/v4"


	//_ "github.com/Monibuca/plugin-ffmpeg"
	// _ "github.com/Monibuca/plugin-cluster"
	// _ "github.com/Monibuca/plugin-gateway/v3"

	// _ "github.com/Monibuca/plugin-gb28181/v3"
	// _ "github.com/Monibuca/plugin-hdl/v4"
	// _ "github.com/Monibuca/plugin-hls/v4"

	// _ "github.com/Monibuca/plugin-jessica/v3"
	// _ "github.com/Monibuca/plugin-logrotate/v3"
	// _ "github.com/Monibuca/plugin-record/v3"
	_ "github.com/Monibuca/plugin-debug/v4"
	_ "github.com/Monibuca/plugin-rtmp/v4"
	// _ "github.com/Monibuca/plugin-rtsp/v3"
	// _ "github.com/Monibuca/plugin-summary"
	// _ "github.com/Monibuca/plugin-ts/v3"
	// _ "github.com/Monibuca/plugin-webrtc/v3"
)

func main() {
	addr := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	go waiter(cancel)
	Run(ctx, *addr)
}

func waiter(cancel context.CancelFunc) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)
	<-sigc
	cancel()
}
