package plugin_crontab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	task "github.com/langhuihui/gotask"
)

// RecordRetryTickTask periodically checks recording status; only one startRecording attempt per slot
type RecordRetryTickTask struct {
	task.TickTask
	cron     *Crontab
	interval time.Duration
}

func (r *RecordRetryTickTask) GetTickInterval() time.Duration {
	return r.interval
}

// Tick:
// 1) if outside slot or stopped -> stop self
// 2) if recording -> keep state
// 3) query stream info; if recording list has expected item -> mark recording
// 4) if not recording -> 周期重试 startRecording（Go 侧用 startAttempted 防热循环）
func (r *RecordRetryTickTask) Tick(any) {
	if r.cron == nil {
		r.Stop(errors.New("record retry task lost cron ref"))
		return
	}
	if r.cron.IsStopped() || r.IsStopped() {
		r.Stop(errors.New("time slot ended or cron stopped"))
		return
	}
	r.cron.Debug("RecordRetryTickTask", "start tick", "")

	now := time.Now()
	// 主任务已停或不在有效时间段，结束自身
	if r.cron.currentSlot == nil || now.After(r.cron.currentSlot.End) {
		r.Stop(errors.New("time slot ended or cron stopped"))
		return
	}

	// 未到开始时间不做处理，等待主调度触发
	if now.Before(r.cron.currentSlot.Start) {
		return
	}

	addr := r.cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}
	url := fmt.Sprintf("http://%s/api/stream/info/%s", addr, r.cron.StreamPath)

	ctx, cancel := r.cron.httpContext(10 * time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		r.cron.Warn("crontab", "err", "build record status request failed", "url", url, "detail", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.cron.Warn("crontab", "err", "query record status failed", "url", url, "detail", err)
		return
	}
	defer resp.Body.Close()

	var info struct {
		Code int `json:"code"`
		Data struct {
			Recording []json.RawMessage `json:"recording"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		r.cron.Warn("RecordRetryTickTask", "err", "parse record status failed", "url", url, "detail", err)
		return
	}
	if r.cron.IsStopped() {
		return
	}
	recordingJSON, _ := json.Marshal(info.Data.Recording)
	r.SetDescription("get recordInfo result.Code", info.Code)
	r.SetDescription("get recordInfo result.Data", string(recordingJSON))
	r.SetDescription("request time", time.Now().Format("2006-01-02 15:04:05"))
	r.SetDescription("startAttempted is", r.cron.startAttempted)

	if info.Code != 0 {
		if info.Code == 2 {
			r.cron.recording = false
			r.cron.startAttempted = false
			r.cron.Debug("RecordRetryTickTask", "start attempted is", r.cron.startAttempted, "info.Code", info.Code)
		} else {
			r.cron.Debug("RecordRetryTickTask", "msg", "record status non-zero code", "code", info.Code, "url", url)
			return
		}
	}

	// recording list check: match filePath/mode if provided
	if len(info.Data.Recording) > 0 {
		type recStatus struct {
			FilePath   string `json:"filePath"`
			PluginName string `json:"pluginName"`
		}
		expectedPath := r.cron.FilePath
		expectedRecordType := r.cron.RecordType
		expectedPlugin := pluginAPIName(expectedRecordType)
		foundMatch := false

		for _, raw := range info.Data.Recording {
			var rec recStatus
			if err := json.Unmarshal(raw, &rec); err != nil {
				continue
			}
			pathOK := expectedPath == "" || rec.FilePath == expectedPath
			// fmp4 开录走 mp4 插件，按 PluginName==mp4 匹配
			recordTypeOK := expectedRecordType == "" || strings.ToLower(rec.PluginName) == expectedPlugin
			if pathOK && recordTypeOK {
				foundMatch = true
				break
			}
		}

		if foundMatch {
			r.cron.recording = true
			r.cron.startAttempted = false
			r.SetDescription("current step", "foundMatch and set recording=true,startAttempted=false")
			r.cron.Info("RecordRetryTickTask", "event", "recording detected", "stream", r.cron.StreamPath, "filePath", expectedPath, "expectedRecordType", expectedRecordType)
			return
		}

		if r.cron.recording {
			r.cron.recording = false
			r.cron.startAttempted = false
			r.cron.Info("RecordRetryTickTask", "event", "recording mismatch, reset", "stream", r.cron.StreamPath, "filePath", expectedPath, "expectedRecordType", expectedRecordType)
		}
	}

	if r.cron.recording {
		r.cron.recording = false
		r.cron.startAttempted = false
		r.cron.Info("RecordRetryTickTask", "event", "recording stopped", "stream", r.cron.StreamPath)
	}

	// 未在录：按 tick 周期再试一次开始录制
	// （主循环负责到点先试一次并长睡；这里负责失败后每隔约 10 秒补试）
	if r.cron.IsStopped() {
		return
	}
	r.cron.Info("RecordRetryTickTask", "event", "no recording, startRecording", "stream", r.cron.StreamPath)
	r.SetDescription("current step", "retry start recording")
	// #region agent log
	debugAgentLog("D", "retry_watcher.go:Tick/retry", "每10秒帮手发现还没在录，再次开始录制", map[string]any{
		"streamPath":     r.cron.StreamPath,
		"startAttempted": r.cron.startAttempted,
	})
	// #endregion
	r.cron.startRecording()
}
