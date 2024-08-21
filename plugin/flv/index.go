package plugin_flv

import (
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"m7s.live/m7s/v5"

	. "m7s.live/m7s/v5/plugin/flv/pkg"
)

type FLVPlugin struct {
	m7s.Plugin
	Path string
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

var _ = m7s.InstallPlugin[FLVPlugin](defaultConfig, NewPuller, NewRecorder)

func (plugin *FLVPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".flv")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	query := r.URL.Query()
	startTimeStr := query.Get("start")
	speedStr := query.Get("speed")
	speed, err := strconv.ParseFloat(speedStr, 64)
	if err != nil {
		speed = 1
	}
	s, err := strconv.Atoi(startTimeStr)
	if err == nil {
		startTime := time.UnixMilli(int64(s))
		var vod Vod
		if err = vod.Init(startTime, filepath.Join(plugin.Path, streamPath)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		vod.SetSpeed(speed)
		err = vod.Run(r.Context())
		return
	}
	var live Live
	live.Subscriber, err = plugin.Subscribe(r.Context(), streamPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "video/x-flv")
	w.Header().Set("Transfer-Encoding", "identity")
	w.WriteHeader(http.StatusOK)
	wto := plugin.GetCommonConf().WriteTimeout
	if hijacker, ok := w.(http.Hijacker); ok && wto > 0 {
		conn, _, _ := hijacker.Hijack()
		conn.SetWriteDeadline(time.Now().Add(wto))
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			conn.SetWriteDeadline(time.Now().Add(wto))
			_, err = flv.WriteTo(conn)
			return
		}
	} else {
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			_, err = flv.WriteTo(w)
			return
		}
		w.(http.Flusher).Flush()
	}
	live.Run()
}
