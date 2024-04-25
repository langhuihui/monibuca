package main

import (
	"context"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/hdl"
	_ "m7s.live/m7s/v5/plugin/rtmp"
	_ "m7s.live/m7s/v5/plugin/webrtc"
)

func main() {
	m7s.Run(context.Background(), "config.yaml")
}
