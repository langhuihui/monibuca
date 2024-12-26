package main

import (
	"context"
	"flag"

	"m7s.live/v5"
	_ "m7s.live/v5/plugin/cascade"

	// _ "m7s.live/v5/plugin/crypto"
	_ "m7s.live/v5/plugin/debug"
	_ "m7s.live/v5/plugin/flv"
	_ "m7s.live/v5/plugin/gb28181"
	_ "m7s.live/v5/plugin/hls"
	_ "m7s.live/v5/plugin/logrotate"
	_ "m7s.live/v5/plugin/monitor"
	_ "m7s.live/v5/plugin/mp4"
	_ "m7s.live/v5/plugin/preview"
	_ "m7s.live/v5/plugin/rtmp"
	_ "m7s.live/v5/plugin/rtsp"
	_ "m7s.live/v5/plugin/sei"
	_ "m7s.live/v5/plugin/snap"
	_ "m7s.live/v5/plugin/srt"
	_ "m7s.live/v5/plugin/stress"
	_ "m7s.live/v5/plugin/transcode"
	_ "m7s.live/v5/plugin/webrtc"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
