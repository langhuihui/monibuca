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
// 4) if not recording and not attempted -> attempt once; otherwise skip
func (r *RecordRetryTickTask) Tick(any) {
	if r.cron == nil {
		r.Stop(errors.New("record retry task lost cron ref"))
		return
	}
	r.cron.Debug("RecordRetryTickTask", "start tick", "")

	now := time.Now()
	// 如果主任务已停止或不在有效时间段，结束自身
	if !r.cron.running || r.cron.currentSlot == nil || now.After(r.cron.currentSlot.End) {
		r.Stop(errors.New("time slot ended or cron stopped"))
		return
	}

	// 未到开始时间不做处理，等待主调度触发
	if now.Before(r.cron.currentSlot.Start) {
		return
	}

	// 组装查询地址
	addr := r.cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}
	url := fmt.Sprintf("http://%s/api/stream/info/%s", addr, r.cron.StreamPath)

	resp, err := http.Get(url)
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
		foundMatch := false

		for _, raw := range info.Data.Recording {
			var rec recStatus
			if err := json.Unmarshal(raw, &rec); err != nil {
				continue
			}
			pathOK := expectedPath == "" || rec.FilePath == expectedPath
			recordTypeOK := expectedRecordType == "" || strings.ToLower(rec.PluginName) == strings.ToLower(expectedRecordType)
			if pathOK && recordTypeOK {
				foundMatch = true
				break
			}
		}

		if foundMatch {
			// mark recording success; allow next attempt after stop
			r.cron.recording = true
			r.cron.startAttempted = false
			r.SetDescription("current step", "foundMatch and set recording=true,startAttempted=false")
			r.cron.Info("RecordRetryTickTask", "event", "recording detected", "stream", r.cron.StreamPath, "filePath", expectedPath, "expectedRecordType", expectedRecordType)
			return
		}

		// recording present but params mismatch -> treat as not matched
		if r.cron.recording {
			r.cron.recording = false
			r.cron.startAttempted = false
			r.cron.Info("RecordRetryTickTask", "event", "recording mismatch, reset", "stream", r.cron.StreamPath, "filePath", expectedPath, "expectedRecordType", expectedRecordType)
		}
	}

	// no recording: if previously recording, reset state
	if r.cron.recording {
		r.cron.recording = false
		// Also reset startAttempted so the next tick can immediately retry.
		// Without this, if foundMatch never ran to reset startAttempted, a
		// publisher reconnect after video-timeout keeps startAttempted=true
		// forever and recording never resumes.
		r.cron.startAttempted = false
		r.cron.Info("RecordRetryTickTask", "event", "recording stopped", "stream", r.cron.StreamPath)
	}

	// not recording and not attempted -> try once; otherwise skip to avoid duplicate subscribers
	if !r.cron.startAttempted {
		r.cron.Info("RecordRetryTickTask", "event", "no recording, first startRecording", "stream", r.cron.StreamPath)
		r.SetDescription("current step", "not startAttempted,start recording")
		r.cron.startRecording()
	} else {
		r.cron.Debug("RecordRetryTickTask", "msg", "startRecording already attempted in this slot", "stream", r.cron.StreamPath)
		r.SetDescription("current step", "has startAttempted,do nothing")
	}
}
