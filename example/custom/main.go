package main

import (
	"context"
	"flag"
	"fmt"
	"m7s.live/v5"
	_ "m7s.live/v5/plugin/console"
	_ "m7s.live/v5/plugin/debug"
	_ "m7s.live/v5/plugin/flv"
	_ "m7s.live/v5/plugin/gb28181"
	_ "m7s.live/v5/plugin/logrotate"
	_ "m7s.live/v5/plugin/monitor"
	_ "m7s.live/v5/plugin/mp4"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	_ "m7s.live/v5/plugin/preview"
	_ "m7s.live/v5/plugin/rtmp"
	_ "m7s.live/v5/plugin/rtsp"
	_ "m7s.live/v5/plugin/sei"
	_ "m7s.live/v5/plugin/srt"
	_ "m7s.live/v5/plugin/stress"
	_ "m7s.live/v5/plugin/transcode"
	_ "m7s.live/v5/plugin/webrtc"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	mp4.CustomFileName = func(job *m7s.RecordJob) string {
		if job.Fragment == 0 {
			return job.FilePath + ".mp4"
		}
		ss := strings.Split(job.StreamPath, "/")
		lastPart := ss[len(ss)-1]
		return filepath.Join(job.FilePath, fmt.Sprintf("%s_%s%s", lastPart, time.Now().Local().Format("2006-01-02-15-04-05"), ".mp4"))
	}
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
