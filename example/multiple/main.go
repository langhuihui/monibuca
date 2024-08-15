package main

import (
	"context"
	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/console"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/logrotate"
	_ "m7s.live/m7s/v5/plugin/rtmp"
	_ "m7s.live/m7s/v5/plugin/rtsp"
	_ "m7s.live/m7s/v5/plugin/stress"
	_ "m7s.live/m7s/v5/plugin/webrtc"
)

func main() {
	ctx := context.Background()
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.AddRootTaskWithContext(ctx, m7s.NewServer("config2.yaml"))
	m7s.Run(ctx, "config1.yaml")
}
