package main

import (
	"context"
	"flag"
	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/console"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/gb28181"
	_ "m7s.live/m7s/v5/plugin/logrotate"
	_ "m7s.live/m7s/v5/plugin/monitor"
	_ "m7s.live/m7s/v5/plugin/mp4"
	_ "m7s.live/m7s/v5/plugin/preview"
	_ "m7s.live/m7s/v5/plugin/rtmp"
	_ "m7s.live/m7s/v5/plugin/rtsp"
	_ "m7s.live/m7s/v5/plugin/stress"
	_ "m7s.live/m7s/v5/plugin/vmlog"
	_ "m7s.live/m7s/v5/plugin/webrtc"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
