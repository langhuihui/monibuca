package main

import (
	"context"
	"flag"

	"m7s.live/engine/v4"
	"m7s.live/engine/v4/util"

	_ "m7s.live/plugin/debug/v4"
	_ "m7s.live/plugin/hdl/v4"
	_ "m7s.live/plugin/hls/v4"
	_ "m7s.live/plugin/jessica/v4"
	_ "m7s.live/plugin/rtmp/v4"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	go util.WaitTerm(cancel)
	engine.Run(ctx, *conf)
}
