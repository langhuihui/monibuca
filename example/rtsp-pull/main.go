package main

import (
	"context"
	"flag"
	"time"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/rtsp"
)

func main() {
	ctx := context.Background()
	var multi bool
	flag.BoolVar(&multi, "multi", false, "debug")
	flag.Parse()
	if multi {
		go m7s.Run(ctx, "config1.yaml")
	}
	time.Sleep(time.Second)
	m7s.NewServer().Run(ctx, "config2.yaml")
}
