package main

import (
	"context"
	"time"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/hdl"
	_ "m7s.live/m7s/v5/plugin/rtmp"
)

func main() {
	ctx := context.Background()
	go m7s.Run(ctx, "config1.yaml")
	time.Sleep(2 * time.Second)
	m7s.NewServer().Run(ctx, "config2.yaml")
}
