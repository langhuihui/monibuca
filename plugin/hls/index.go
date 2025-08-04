package plugin_hls

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unsafe"

	_ "embed"

	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	hls "m7s.live/v5/plugin/hls/pkg"
)

var _ = m7s.InstallPlugin[HLSPlugin](m7s.PluginMeta{
	NewTransformer: hls.NewTransform,
	NewRecorder:    hls.NewRecorder,
	NewPuller:      hls.NewPuller,
	NewPullProxy:   m7s.NewHTTPPullPorxy,
})

//go:embed hls.js.zip
var hls_js []byte
var zipReader *zip.Reader

type HLSPlugin struct {
	m7s.Plugin
}

func init() {
	zipReader, _ = zip.NewReader(bytes.NewReader(hls_js), int64(len(hls_js)))
}

func (p *HLSPlugin) Start() (err error) {
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

func (p *HLSPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/vod/{streamPath...}":              p.vod,
		"/download/{streamPath...}":         p.download,
		"/api/record/start/{streamPath...}": p.API_record_start,
		"/api/record/stop/{id}":             p.API_record_stop,
	}
}

func (config *HLSPlugin) vod(w http.ResponseWriter, r *http.Request) {
	recordType := "ts"
	if r.PathValue("streamPath") == "mp4.m3u8" {
		recordType = "mp4"
	} else if r.PathValue("streamPath") == "fmp4.m3u8" {
		recordType = "fmp4"
	}
	query := r.URL.Query()
	fileName := query.Get("streamPath")
	if fileName == "" {
		fileName = r.PathValue("streamPath")
	}
	waitTimeout, err := time.ParseDuration(query.Get("timeout"))
	if err == nil {
		config.Debug("request", "fileName", fileName, "timeout", waitTimeout)
	} else {
		waitTimeout = time.Second * 10
	}
	// waitStart := time.Now()
	if strings.HasSuffix(r.URL.Path, ".m3u8") {
		w.Header().Add("Content-Type", "application/vnd.apple.mpegurl")
		streamPath := strings.TrimSuffix(fileName, ".m3u8")
		// If memory lookup failed or returned empty, try database
		startTime, endTime, _ := util.TimeRangeQueryParse(query)
		if !startTime.IsZero() {
			if config.DB != nil {
				var records []m7s.RecordStream
				if recordType == "fmp4" {
					query := `stream_path = ? AND type = ? AND start_time IS NOT NULL AND end_time IS NOT NULL AND ? <= end_time AND ? >= start_time`
					config.DB.Where(query, streamPath, "mp4", startTime, endTime).Find(&records)
					if len(records) == 0 {
						return
					}
					playlist := hls.Playlist{
						Version:        7,
						Sequence:       0,
						Targetduration: 90,
					}
					var plBuffer util.Buffer
					playlist.Writer = &plBuffer
					playlist.Init()

					for _, record := range records {
						playlist.WriteInf(hls.PlaylistInf{
							Duration: float64(record.Duration) / 1000,
							URL:      fmt.Sprintf("/mp4/download/%s.fmp4?id=%d", streamPath, record.ID),
							Title:    record.StartTime.Format(time.RFC3339),
						})
					}
					plBuffer.WriteString("#EXT-X-ENDLIST\n")
					w.Write(plBuffer)
					return
				} else if recordType == "ts" {
					playlist := hls.Playlist{
						Version:        3,
						Sequence:       0,
						Targetduration: 10,
					}
					var plBuffer util.Buffer
					playlist.Writer = &plBuffer
					playlist.Init()
					for i := startTime; i.Before(endTime); i = i.Add(10 * time.Second) {
						playlist.WriteInf(hls.PlaylistInf{
							Duration: 10,
							URL:      fmt.Sprintf("/hls/download/%s.ts?start=%d&end=%d", streamPath, i.Unix(), i.Add(10*time.Second).Unix()),
							Title:    i.Format(time.RFC3339),
						})
					}
					plBuffer.WriteString("#EXT-X-ENDLIST\n")
					w.Write(plBuffer)
					return
				}
				query := `stream_path = ? AND type = ? AND start_time IS NOT NULL AND end_time IS NOT NULL AND ? <= end_time AND ? >= start_time`
				config.DB.Where(query, streamPath, recordType, startTime, endTime).Find(&records)
				if len(records) > 0 {
					playlist := hls.Playlist{
						Version:        7,
						Sequence:       0,
						Targetduration: 90,
					}
					var plBuffer util.Buffer
					playlist.Writer = &plBuffer
					playlist.Init()

					for _, record := range records {
						playlist.WriteInf(hls.PlaylistInf{
							Duration: float64(record.Duration) / 1000,
							URL:      record.FilePath,
						})
					}
					plBuffer.WriteString("#EXT-X-ENDLIST\n")
					w.Write(plBuffer)
					return
				}
			}
		}

		// if v, ok := hls.MemoryM3u8.Load(streamPath); ok && v.(string) != "" {
		// 	w.Write([]byte(v.(string)))
		// 	return
		// }
		// for {
		// 	if v, ok := hls.MemoryM3u8.Load(streamPath); ok && v.(string) != "" {
		// 		w.Write([]byte(v.(string)))
		// 		return
		// 	}
		// 	if waitTimeout > 0 && time.Since(waitStart) < waitTimeout {
		// 		config.Server.OnSubscribe(streamPath, r.URL.Query())
		// 		time.Sleep(time.Second)
		// 		continue
		// 	} else {
		// 		break
		// 	}
		// }
	} else if strings.HasSuffix(r.URL.Path, ".mp4") {
		w.Header().Add("Content-Type", "video/mp4") //video/mp4
		data, err := os.ReadFile(r.PathValue("streamPath"))
		if err == nil {
			w.Write(data)
			return
		}
		// 	streamPath := path.Dir(fileName)
		// 	tsData, ok := hls.MemoryTs.Load(streamPath)
		// 	if !ok {
		// 		tsData, ok = hls.MemoryTs.Load(path.Dir(streamPath))
		// 	}
		// 	if ok {
		// 		if tsData, ok := tsData.(hls.TsCacher).GetTs(fileName); ok {
		// 			switch v := tsData.(type) {
		// 			case *hls.TsInMemory:
		// 				v.WriteTo(w)
		// 			case util.Buffer:
		// 				w.Write(v)
		// 			}
		// 			return
		// 		}
		// 	}
	}
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
				query := `stream_path = ? AND type = 'ts' AND start_time IS NOT NULL AND end_time IS NOT NULL AND ? <= end_time AND ? >= start_time`
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
							URL:      record.FilePath,
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
		data, err := os.ReadFile(fileName)
		if err == nil {
			w.Write(data)
			return
		}
		parts := strings.Split(fileName, "/")
		filePath := strings.Join(parts[1:], "/")
		data, err = os.ReadFile(filePath)
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
		http.ServeFileFS(w, r, zipReader, r.URL.Path)
	}
}

func (conf *HLSPlugin) API_record_start(w http.ResponseWriter, r *http.Request) {
	var recordExists bool
	var filePath = "."
	var fragment = time.Minute
	query := r.URL.Query()
	streamPath := r.PathValue("streamPath")
	if streamPath == "" {
		http.Error(w, "streamPath is required", http.StatusBadRequest)
		return
	}
	if query.Get("fragment") != "" {
		fragment, _ = time.ParseDuration(query.Get("fragment"))
	}
	if query.Get("filePath") != "" {
		filePath = query.Get("filePath")
	}
	_, recordExists = conf.Server.Records.Find(func(job *m7s.RecordJob) bool {
		return job.StreamPath == streamPath && job.RecConf.FilePath == filePath
	})
	if recordExists {
		http.Error(w, "record already exists", http.StatusBadRequest)
		return
	}
	pub, ok := conf.Server.Streams.SafeGet(streamPath)
	if !ok {
		http.Error(w, "stream not found", http.StatusNotFound)
		return
	}
	stream := pub
	recordConf := config.Record{
		Append:   false,
		Fragment: fragment,
		FilePath: filePath,
	}
	job := conf.Server.Record(stream, recordConf, nil)
	util.ReturnValue(uint64(uintptr(unsafe.Pointer(job.GetTask()))), w, r)
}

func (conf *HLSPlugin) API_record_stop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ptr, err := strconv.Atoi(id)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	t := task.FromPointer(uintptr(ptr))
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	t.Stop(task.ErrStopByUser)
}
