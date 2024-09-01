package main

import (
	"context"
	"flag"
	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/cascade"
	_ "m7s.live/m7s/v5/plugin/console"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/logrotate"
	_ "m7s.live/m7s/v5/plugin/monitor"
	_ "m7s.live/m7s/v5/plugin/rtmp"
	_ "m7s.live/m7s/v5/plugin/rtsp"
	_ "m7s.live/m7s/v5/plugin/stress"
	_ "m7s.live/m7s/v5/plugin/webrtc"
	"path/filepath"
)

func main() {
	ctx := context.Background()
	conf := flag.String("c", "", "config file dir")
	flag.Parse()
	go m7s.Run(ctx, filepath.Join(*conf, "config2.yaml"))
	m7s.Run(ctx, filepath.Join(*conf, "config1.yaml"))
}
