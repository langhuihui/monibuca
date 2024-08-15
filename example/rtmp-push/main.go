package main

import (
	"context"
	"flag"
	"time"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/rtmp"
)

func main() {
	ctx := context.Background()
	var multi bool
	flag.BoolVar(&multi, "multi", false, "debug")
	flag.Parse()
	if multi {
		m7s.AddRootTaskWithContext(ctx, m7s.NewServer("config2.yaml"))
	}
	time.Sleep(time.Second)
	m7s.Run(ctx, "config1.yaml")
}
