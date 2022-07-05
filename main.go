package main

//go:generate go run gen.go $debug

/*
                                              
███╗   ███╗ ██████╗ ███╗   ██╗██╗██████╗ ██╗   ██╗ ██████╗ █████╗ 
████╗ ████║██╔═══██╗████╗  ██║██║██╔══██╗██║   ██║██╔════╝██╔══██╗
██╔████╔██║██║   ██║██╔██╗ ██║██║██████╔╝██║   ██║██║     ███████║
██║╚██╔╝██║██║   ██║██║╚██╗██║██║██╔══██╗██║   ██║██║     ██╔══██║
██║ ╚═╝ ██║╚██████╔╝██║ ╚████║██║██████╔╝╚██████╔╝╚██████╗██║  ██║
╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝╚═════╝  ╚═════╝  ╚═════╝╚═╝  ╚═╝
                                                                                                                            
The live stream server for Go
(c) dexter 2019-present
*/

import (
	"context"
	"flag"

	"m7s.live/engine/v4"
	"m7s.live/engine/v4/util"

	_ "m7s.live/plugin/debug/v4"
	_ "m7s.live/plugin/gb28181/v4"
	_ "m7s.live/plugin/hdl/v4"
	_ "m7s.live/plugin/hls/v4"
	_ "m7s.live/plugin/hook/v4"
	_ "m7s.live/plugin/jessica/v4"
	_ "m7s.live/plugin/logrotate/v4"
	_ "m7s.live/plugin/preview/v4"
	_ "m7s.live/plugin/record/v4"
	_ "m7s.live/plugin/room/v4"
	_ "m7s.live/plugin/rtmp/v4"
	_ "m7s.live/plugin/rtsp/v4"
	_ "m7s.live/plugin/webrtc/v4"
	_ "m7s.live/plugin/webtransport/v4"
	_ "m7s.live/plugin/snap/v4"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	go util.WaitTerm(cancel)
	engine.Run(ctx, *conf)
}
