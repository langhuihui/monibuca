package plugin_hls

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/util"
	hls "m7s.live/v5/plugin/hls/pkg"
)

var _ = m7s.InstallPlugin[HLSPlugin](hls.NewTransform, hls.NewRecorder)

//go:embed hls.js
var hls_js embed.FS

type HLSPlugin struct {
	m7s.Plugin
}

func (p *HLSPlugin) OnInit() (err error) {
	_, port, _ := strings.Cut(p.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		p.PlayAddr = append(p.PlayAddr, "http://{hostName}/hls/{streamPath}.m3u8")
	} else if port != "" {
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("http://{hostName}:%s/hls/{streamPath}.m3u8", port))
	}
	_, port, _ = strings.Cut(p.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		p.PlayAddr = append(p.PlayAddr, "https://{hostName}/hls/{streamPath}.m3u8")
	} else if port != "" {
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("https://{hostName}:%s/hls/{streamPath}.m3u8", port))
	}
	return
}

func (p *HLSPlugin) OnPullProxyAdd(pullProxy *m7s.PullProxy) any {
	d := &m7s.HTTPPullProxy{}
	d.PullProxy = pullProxy
	d.Plugin = &p.Plugin
	return d
}

func (config *HLSPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fileName := strings.TrimPrefix(r.URL.Path, "/")
	query := r.URL.Query()
	waitTimeout, err := time.ParseDuration(query.Get("timeout"))
	if err == nil {
		config.Debug("request", "fileName", fileName, "timeout", waitTimeout)
	} else {
		waitTimeout = time.Second * 10
	}
	waitStart := time.Now()
	if strings.HasSuffix(r.URL.Path, ".m3u8") {
		w.Header().Add("Content-Type", "application/vnd.apple.mpegurl")
		streamPath := strings.TrimSuffix(fileName, ".m3u8")
		// If memory lookup failed or returned empty, try database
		startTime, endTime, _ := util.TimeRangeQueryParse(r.URL.Query())
		if !startTime.IsZero() {
			if config.DB != nil {
				var records []m7s.RecordStream
				query := `stream_path = ? AND type = 'hls' AND start_time IS NOT NULL AND end_time IS NOT NULL AND ? <= end_time AND ? >= start_time`
				config.DB.Where(query, streamPath, startTime, endTime).Find(&records)

				if len(records) > 0 {
					playlist := hls.Playlist{
						Version:        3,
						Sequence:       0,
						Targetduration: 90,
					}
					var plBuffer util.Buffer
					playlist.Writer = &plBuffer
					playlist.Init()

					for _, record := range records {
						duration := record.EndTime.Sub(record.StartTime).Seconds()
						playlist.WriteInf(hls.PlaylistInf{
							Duration: duration,
							Title:    path.Base(record.FilePath),
							FilePath: record.FilePath,
						})
					}
					plBuffer.WriteString("#EXT-X-ENDLIST\n")
					w.Write(plBuffer)
					return
				}
			}
		}

		if v, ok := hls.MemoryM3u8.Load(streamPath); ok && v.(string) != "" {
			w.Write([]byte(v.(string)))
			return
		}
		for {
			if v, ok := hls.MemoryM3u8.Load(streamPath); ok && v.(string) != "" {
				w.Write([]byte(v.(string)))
				return
			}
			if waitTimeout > 0 && time.Since(waitStart) < waitTimeout {
				config.Server.OnSubscribe(streamPath, r.URL.Query())
				time.Sleep(time.Second)
				continue
			} else {
				break
			}
		}
	} else if strings.HasSuffix(r.URL.Path, ".ts") {
		w.Header().Add("Content-Type", "video/mp2t") //video/mp2t
		parts := strings.Split(fileName, "/")
		filePath := strings.Join(parts[1:], "/")
		data, err := os.ReadFile(filePath)
		if err == nil {
			w.Write(data)
			return
		}
		streamPath := path.Dir(fileName)
		tsData, ok := hls.MemoryTs.Load(streamPath)
		if !ok {
			tsData, ok = hls.MemoryTs.Load(path.Dir(streamPath))
		}
		if ok {
			if tsData, ok := tsData.(hls.TsCacher).GetTs(fileName); ok {
				switch v := tsData.(type) {
				case *hls.TsInMemory:
					v.WriteTo(w)
				case util.Buffer:
					w.Write(v)
				}
				return
			}
		}
	} else {
		f, err := hls_js.ReadFile("hls.js/" + fileName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			w.Write(f)
		}
		// if file, err := hls_js.Open(fileName); err == nil {
		// 	defer file.Close()
		// 	if info, err := file.Stat(); err == nil {
		// 		http.ServeContent(w, r, fileName, info.ModTime(), file)
		// 	}
		// } else {
		// 	http.NotFound(w, r)
		// }
	}
}
