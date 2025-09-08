package main

import (
	"context"
	"flag"

	"m7s.live/v5"
	_ "m7s.live/v5/plugin/cascade"
	_ "m7s.live/v5/plugin/debug"
	_ "m7s.live/v5/plugin/flv"
	_ "m7s.live/v5/plugin/logrotate"
	_ "m7s.live/v5/plugin/rtmp"
	_ "m7s.live/v5/plugin/rtsp"
	_ "m7s.live/v5/plugin/test"
	_ "m7s.live/v5/plugin/webrtc"
)

func main() {
	ctx := context.Background()
	conf1 := flag.String("c1", "", "config1 file")
	conf2 := flag.String("c2", "", "config2 file")
	flag.Parse()
	go m7s.Run(ctx, *conf2)
	m7s.Run(ctx, *conf1)
}
