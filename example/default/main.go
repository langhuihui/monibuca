package main

import (
	"context"
	"flag"

	"m7s.live/pro"
	_ "m7s.live/pro/plugin/console"
	_ "m7s.live/pro/plugin/debug"
	_ "m7s.live/pro/plugin/flv"
	_ "m7s.live/pro/plugin/gb28181"
	_ "m7s.live/pro/plugin/logrotate"
	_ "m7s.live/pro/plugin/monitor"
	_ "m7s.live/pro/plugin/mp4"
	_ "m7s.live/pro/plugin/preview"
	_ "m7s.live/pro/plugin/rtmp"
	_ "m7s.live/pro/plugin/rtsp"
	_ "m7s.live/pro/plugin/sei"
	_ "m7s.live/pro/plugin/srt"
	_ "m7s.live/pro/plugin/stress"
	_ "m7s.live/pro/plugin/transcode"
	_ "m7s.live/pro/plugin/webrtc"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
