package main

import (
	"context"
	"flag"
	"m7s.live/pro"
	_ "m7s.live/pro/plugin/cascade"
	_ "m7s.live/pro/plugin/console"
	_ "m7s.live/pro/plugin/debug"
	_ "m7s.live/pro/plugin/flv"
	_ "m7s.live/pro/plugin/logrotate"
	_ "m7s.live/pro/plugin/monitor"
	_ "m7s.live/pro/plugin/rtmp"
	_ "m7s.live/pro/plugin/rtsp"
	_ "m7s.live/pro/plugin/stress"
	_ "m7s.live/pro/plugin/webrtc"
	"path/filepath"
)

func main() {
	ctx := context.Background()
	conf := flag.String("c", "", "config file dir")
	flag.Parse()
	go m7s.Run(ctx, filepath.Join(*conf, "config2.yaml"))
	m7s.Run(ctx, filepath.Join(*conf, "config1.yaml"))
}
