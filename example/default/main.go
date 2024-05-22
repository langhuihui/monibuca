package main

import (
	"context"
	"errors"
	"flag"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	_ "m7s.live/m7s/v5/plugin/console"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/hdl"
	_ "m7s.live/m7s/v5/plugin/logrotate"
	_ "m7s.live/m7s/v5/plugin/rtmp"
	_ "m7s.live/m7s/v5/plugin/webrtc"
	"strings"
)

func init() {
	//全局推流鉴权
	m7s.DefaultServer.OnAuthPubs["RTMP"] = func(p *util.Promise[*m7s.Publisher]) {
		var pub = p.Value
		if strings.Contains(pub.StreamPath, "20A222800207-2") {
			p.Fulfill(nil)
		} else {
			p.Fulfill(errors.New("auth failed"))
		}
	}
	//全局播放鉴权
	m7s.DefaultServer.OnAuthSubs["RTMP"] = func(p *util.Promise[*m7s.Subscriber]) {
		var sub = p.Value
		if strings.Contains(sub.StreamPath, "20A222800207-22") {
			p.Fulfill(nil)
		} else {
			p.Fulfill(errors.New("auth failed"))
		}
	}
}

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
