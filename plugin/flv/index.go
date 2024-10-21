package plugin_flv

import (
	"net"
	"net/http"
	"strings"
	"time"

	"m7s.live/v5"

	"m7s.live/v5/pkg/task"
	. "m7s.live/v5/plugin/flv/pkg"
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
	//query := r.URL.Query()
	//speedStr := query.Get("speed")
	//speed, err := strconv.ParseFloat(speedStr, 64)
	var err error
	defer func() {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}()
	//if err != nil {
	//	speed = 1
	//}
	//if startTime, err := util.TimeQueryParse(query.Get("start")); err == nil {
	//	var vod Vod
	//	if err = vod.Init(startTime, filepath.Join(plugin.Path, streamPath)); err != nil {
	//		http.Error(w, err.Error(), http.StatusBadRequest)
	//		return
	//	}
	//	vod.Writer = w
	//	vod.SetSpeed(speed)
	//	plugin.Info("vod start", "streamPath", streamPath, "startTime", startTime, "speed", speed)
	//	err = vod.Run(r.Context())
	//	plugin.Info("vod done", "streamPath", streamPath, "err", err)
	//	return
	//}
	var live Live
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}
	live.Subscriber, err = plugin.Subscribe(r.Context(), streamPath)
	if err != nil {
		return
	}
	var conn net.Conn
	conn, err = live.Subscriber.CheckWebSocket(w, r)
	if err != nil {
		return
	}
	wto := plugin.GetCommonConf().WriteTimeout
	if conn == nil {
		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Transfer-Encoding", "identity")
		w.WriteHeader(http.StatusOK)
		if hijacker, ok := w.(http.Hijacker); ok && wto > 0 {
			conn, _, _ = hijacker.Hijack()
			conn.SetWriteDeadline(time.Now().Add(wto))
		}
	}
	if conn == nil {
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			_, err = flv.WriteTo(w)
			return
		}
		w.(http.Flusher).Flush()
	} else {
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			conn.SetWriteDeadline(time.Now().Add(wto))
			_, err = flv.WriteTo(conn)
			return
		}
	}
	err = live.Run()
}

func (plugin *FLVPlugin) OnDeviceAdd(device *m7s.Device) (ret task.ITask) {
	d := &FLVDevice{}
	d.Device = device
	d.Plugin = &plugin.Plugin
	return d
}
