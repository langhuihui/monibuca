package plugin_crontab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5/plugin/crontab/pkg"
)

// TimeSlot describes a recording window
type TimeSlot struct {
	Start time.Time // start time
	End   time.Time // end time
}

// Crontab scheduler
// Confirmed via 寸止 / BUG-023 S3：停机统一走 gotask Context/Done()，不再自建 stop channel
type Crontab struct {
	task.Work
	ctp *CrontabPlugin
	*pkg.RecordPlan
	*pkg.RecordPlanStream

	location       *time.Location
	timer          *time.Timer
	currentSlot    *TimeSlot // current slot
	recording      bool      // currently recording
	startAttempted bool      // 本时段已发起过开录（阻止 Go 热循环；周期重试交给 RetryTick）
	retryTask      *RecordRetryTickTask
	recordMu       sync.Mutex // 串行化 start/stop，避免与 RetryTick 并发双打
}

func (cron *Crontab) GetKey() string {
	return cron.StreamPath + "_" + cron.RecordType
}

// 初始化
func (cron *Crontab) Start() (err error) {
	cron.Info("crontab", "event", "plugin start")

	if cron.location == nil {
		cron.location = time.Local
	}

	cron.SetDescription("streampath", cron.StreamPath)
	cron.SetDescription("planid", cron.PlanID)
	cron.SetDescription("recording status", cron.recording)
	return nil
}

// 阻塞运行：所有等待路径均 select cron.Done()，Stop/remove 后调度退出
func (cron *Crontab) Go() error {
	cron.Info("crontab", "event", "plugin running")
	cron.Info("crontab", "event", "scheduler start")

	for {
		if cron.IsStopped() {
			cron.Info("crontab", "event", "scheduler stop")
			return nil
		}

		now := time.Now().In(cron.location)

		// 时段结束：停录
		if cron.recording && cron.currentSlot != nil &&
			(now.Equal(cron.currentSlot.End) || now.After(cron.currentSlot.End)) {
			cron.stopRecording()
			continue
		}

		var nextEvent time.Time

		if cron.recording {
			nextEvent = cron.currentSlot.End
		} else {
			var nextSlot *TimeSlot
			if cron.currentSlot != nil && now.After(cron.currentSlot.Start) && now.Before(cron.currentSlot.End) {
				nextSlot = cron.currentSlot
				cron.Debug("crontab", "msg", "reuse current slot", "start", nextSlot.Start.Format("2006-01-02 15:04:05"), "end", nextSlot.End.Format("2006-01-02 15:04:05"))
				cron.ensureRetryWatcher()
			} else {
				nextSlot = cron.getNextTimeSlot()
				if nextSlot == nil {
					cron.Info("crontab", "event", "no plan", "action", "wait 1h")
					if e := cron.waitDuration(1 * time.Hour); e != nil {
						cron.Info("crontab", "event", "scheduler stop")
						return e
					}
					continue
				}

				if cron.currentSlot == nil || !nextSlot.Start.Equal(cron.currentSlot.Start) || !nextSlot.End.Equal(cron.currentSlot.End) {
					cron.Info("crontab", "into cron.currentSlot == nil || !nextSlot.Start.Equal(cron.currentSlot.Start) || !nextSlot.End.Equal(cron.currentSlot.End)", "")
					cron.currentSlot = nextSlot
					cron.startAttempted = false
					if cron.retryTask != nil {
						cron.retryTask.Stop(errors.New("switch time slot"))
						cron.retryTask = nil
					}
				}
			}

			nextEvent = nextSlot.Start

			// 已到开录点：首次由 Go 触发；之后交给 RetryTick，主循环等到段末，避免 continue 空转
			if now.Equal(nextEvent) || now.After(nextEvent) {
				if !cron.startAttempted {
					cron.startRecording()
				} else {
					cron.Debug("crontab", "msg", "startRecording already attempted in this slot, wait RetryTick")
				}
				if cron.IsStopped() {
					cron.Info("crontab", "event", "scheduler stop")
					return nil
				}
				// 未在录：长睡前必须挂上「每 10 秒再试」的帮手。
				// 否则开始录制一旦失败（例如重启时流还不存在返回 404），主循环会等到时段结束；
				// 全天计划的结束时间约等于 100 年后，等于永久不再试。
				// ensureRetryWatcher 内部有「已有就不重复建」，可安全多调。
				if !cron.recording && cron.currentSlot != nil {
					// #region agent log
					debugAgentLog("B", "crontab.go:Go/beforeWaitUntil", "长睡前确保已挂上每10秒再试帮手", map[string]any{
						"streamPath":     cron.StreamPath,
						"recording":      cron.recording,
						"startAttempted": cron.startAttempted,
						"hasRetryTask":   cron.retryTask != nil,
						"slotEnd":        cron.currentSlot.End.Format(time.RFC3339),
					})
					// #endregion
					cron.ensureRetryWatcher()
					if e := cron.waitUntil(cron.currentSlot.End); e != nil {
						cron.Info("crontab", "event", "scheduler stop")
						return e
					}
				}
				continue
			}
		}

		waitDuration := nextEvent.Sub(now)
		if waitDuration <= 0 {
			if !cron.recording {
				if !cron.startAttempted {
					cron.startRecording()
				} else {
					cron.Debug("crontab", "msg", "startRecording already attempted in this slot2")
				}
			} else {
				cron.stopRecording()
			}
			continue
		}

		if !cron.recording {
			cron.Info("crontab", "next_start", nextEvent, "wait", waitDuration)
			cron.SetDescription("current step", "wait next start "+nextEvent.Format("2006-01-02 15:04:05"))
		} else {
			cron.Info("crontab", "next_end", nextEvent, "wait", waitDuration)
			cron.SetDescription("current step", "wait next stop "+nextEvent.Format("2006-01-02 15:04:05"))
		}

		if e := cron.waitDuration(waitDuration); e != nil {
			cron.Info("crontab", "event", "scheduler stop")
			return e
		}

		if cron.IsStopped() {
			cron.Info("crontab", "event", "scheduler stop")
			return nil
		}
		if !cron.recording {
			if !cron.startAttempted {
				cron.startRecording()
			} else {
				cron.Debug("crontab", "msg", "startRecording already attempted in this slot3")
			}
		} else {
			cron.stopRecording()
		}
	}
}

// waitDuration 可被任务 Stop 打断；返回非 nil 表示已停止
func (cron *Crontab) waitDuration(d time.Duration) error {
	if d <= 0 {
		if cron.IsStopped() {
			return cron.StopReason()
		}
		return nil
	}
	timer := time.NewTimer(d)
	cron.timer = timer
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if cron.timer == timer {
			cron.timer = nil
		}
	}()
	select {
	case <-timer.C:
		return nil
	case <-cron.Done():
		return cron.StopReason()
	}
}

func (cron *Crontab) waitUntil(deadline time.Time) error {
	return cron.waitDuration(time.Until(deadline))
}

// Dispose：Stop 已 cancel Context；此处只做资源收尾
func (cron *Crontab) Dispose() {
	if cron.timer != nil {
		cron.timer.Stop()
		cron.timer = nil
	}
	if cron.retryTask != nil {
		cron.retryTask.Stop(errors.New("crontab disposed"))
		cron.retryTask = nil
	}
	// 若仍在录，尽力通知停止（任务 context 已取消，使用独立短超时）
	if cron.recording {
		cron.stopRecording()
	}
}

// 获取下一个时间段
func (cron *Crontab) getNextTimeSlot() *TimeSlot {
	if cron.RecordPlan == nil || !cron.RecordPlan.Enable || cron.RecordPlan.Plan == "" {
		return nil // no valid plan
	}
	plan := cron.RecordPlan.Plan
	if len(plan) != 168 {
		cron.Error("crontab", "err", "invalid plan format", "plan", plan)
		return nil
	}

	now := time.Now().In(cron.location)
	start, end, ok := nextSlotRange(plan, now, cron.location)
	if !ok {
		cron.Debug("crontab", "msg", "no valid slot found")
		return nil
	}

	cron.Debug("crontab", "msg", "next slot", "start", start.Format("2006-01-02 15:04:05"), "end", end.Format("2006-01-02 15:04:05"))
	return &TimeSlot{
		Start: start,
		End:   end,
	}
}

// nextSlotRange 是 crontab 内部使用的核心时间段计算逻辑。
// 入参为 168 位计划字符串（周日0点开始），返回从 now 起最近的一个连续录制时间段（支持跨天）。
// 规则与 api_test.go 中的单元测试一致。
func nextSlotRange(plan string, now time.Time, loc *time.Location) (time.Time, time.Time, bool) {
	if len(plan) != 168 {
		return time.Time{}, time.Time{}, false
	}

	localNow := now.In(loc)

	// 特殊情况：整周全为 '1'，视为 7x24 小时永远录制。
	// 这里返回 [now, now+100年]，等价于“当前起长期有效”，避免在 0 点等边界强行 stop。
	if !strings.Contains(plan, "0") {
		return localNow, localNow.AddDate(100, 0, 0), true
	}

	currentWeekday := int(localNow.Weekday()) // 0=Sunday
	currentHour := localNow.Hour()

	currentIndex := currentWeekday*24 + currentHour // [0,167]
	currentHourStart := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		currentHour, 0, 0, 0, loc,
	)

	// 安全取模
	mod := func(i int) int {
		i %= 168
		if i < 0 {
			i += 168
		}
		return i
	}

	// 找到包含 idx 的最大连续 1 段 [startIdx, endIdx)
	findRun := func(idx int) (startIdx, endIdx int) {
		startIdx, endIdx = idx, idx+1

		// 向前扩展，最多一周
		for j := idx - 1; j >= idx-167; j-- {
			if plan[mod(j)] != '1' {
				break
			}
			startIdx--
			if endIdx-startIdx >= 168 {
				break
			}
		}
		// 向后扩展，最多一周
		for j := idx + 1; j < idx+168 && j-startIdx < 168; j++ {
			if plan[mod(j)] != '1' {
				break
			}
			endIdx++
		}
		return
	}

	// 1. 当前小时在某个录制段内
	if plan[mod(currentIndex)] == '1' {
		startIdx, endIdx := findRun(currentIndex)
		startTime := currentHourStart.Add(time.Duration(startIdx-currentIndex) * time.Hour).In(loc)
		endTime := currentHourStart.Add(time.Duration(endIdx-currentIndex) * time.Hour).In(loc)

		// 如果距离结束还有 30 秒以上，则返回当前整段
		if localNow.Before(endTime.Add(-30 * time.Second)) {
			return startTime, endTime, true
		}

		// 否则跳过当前段，从 endIdx 之后开始找下一段
		searchFrom := endIdx
		for offset := 0; offset < 168; offset++ {
			idx := searchFrom + offset
			if plan[mod(idx)] == '1' && plan[mod(idx-1)] != '1' {
				s, e := findRun(idx)
				ns := currentHourStart.Add(time.Duration(s-currentIndex) * time.Hour).In(loc)
				ne := currentHourStart.Add(time.Duration(e-currentIndex) * time.Hour).In(loc)
				return ns, ne, true
			}
		}
		return time.Time{}, time.Time{}, false
	}

	// 2. 当前不在录制段内：从下一小时开始扫描
	searchFrom := currentIndex + 1
	for offset := 0; offset < 168; offset++ {
		idx := searchFrom + offset
		if plan[mod(idx)] == '1' && plan[mod(idx-1)] != '1' {
			startIdx, endIdx := findRun(idx)
			startTime := currentHourStart.Add(time.Duration(startIdx-currentIndex) * time.Hour).In(loc)
			endTime := currentHourStart.Add(time.Duration(endIdx-currentIndex) * time.Hour).In(loc)
			return startTime, endTime, true
		}
	}

	return time.Time{}, time.Time{}, false
}

// httpContext 调度存活时派生自任务 Context；Dispose 停录时用独立短超时
func (cron *Crontab) httpContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	if cron.IsStopped() {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(cron, timeout)
}

// 开始录制
func (cron *Crontab) startRecording() {
	cron.recordMu.Lock()
	if cron.recording || cron.IsStopped() || cron.currentSlot == nil {
		cron.recordMu.Unlock()
		return
	}
	// 先标记已尝试，防止 Go 失败热循环；周期重试由 RecordRetryTick 负责
	cron.startAttempted = true
	planName := cron.RecordPlan.Name
	slotEnd := cron.currentSlot.End
	fragment := cron.Fragment
	filePath := cron.FilePath
	recordType := cron.RecordType
	streamPath := cron.StreamPath
	cron.recordMu.Unlock()

	cron.Debug("crontab", "before send record post,set cron.startAttempted", true)
	now := time.Now().In(cron.location)
	cron.Info("crontab", "event", "start recording", "plan", planName, "time", now, "plan_end", slotEnd)

	reqBody := map[string]string{
		"fragment": fragment,
		"filePath": filePath,
		"mode":     "auto",
	}
	// Confirmed via 寸止: REQ-MP4-002 方案 B — record_type 为 mp4/fmp4 时写入 body.type
	pluginName := pluginAPIName(recordType)
	if pluginName == "mp4" {
		t := strings.ToLower(strings.TrimSpace(recordType))
		if t == "" {
			t = "mp4"
		}
		reqBody["type"] = t
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		cron.Error("crontab", "err", "build request body failed", "detail", err)
		// 失败也要挂上每 10 秒再试的帮手，否则主循环长睡后无人再试
		cron.ensureRetryWatcher()
		return
	}

	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	ctx, cancel := cron.httpContext(10 * time.Second)
	defer cancel()

	startURL := fmt.Sprintf("http://%s/%s/api/start/%s", addr, pluginName, streamPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		cron.Error("crontab", "err", "build start request failed", "detail", err)
		cron.ensureRetryWatcher()
		return
	}
	req.Header.Set("Content-Type", "application/json")

	cron.Debug("crontab", "record_request_url", startURL, "body", string(jsonBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cron.Error("crontab", "err", "start recording failed", "detail", err)
		// #region agent log
		debugAgentLog("A", "crontab.go:startRecording/httpErr", "开始录制请求失败，挂上每10秒再试帮手", map[string]any{
			"streamPath":     streamPath,
			"err":            err.Error(),
			"hasRetryBefore": cron.retryTask != nil,
		})
		// #endregion
		cron.ensureRetryWatcher()
		return
	}
	defer resp.Body.Close()
	cron.SetDescription("response.StatusCode", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		cron.Error("crontab", "err", "start recording failed", "status", resp.StatusCode)
		// #region agent log
		debugAgentLog("A", "crontab.go:startRecording/non200", "开始录制返回非200（含404），挂上每10秒再试帮手", map[string]any{
			"streamPath":     streamPath,
			"status":         resp.StatusCode,
			"hasRetryBefore": cron.retryTask != nil,
		})
		// #endregion
		// 典型场景：服务刚重启，拉流代理还没把流拉起来，接口返回 404。
		// 以前只有成功才挂帮手，导致 404 后永久不再试。
		cron.ensureRetryWatcher()
		return
	}

	cron.recordMu.Lock()
	defer cron.recordMu.Unlock()
	if cron.IsStopped() || cron.recording {
		return
	}
	cron.recording = true
	cron.SetDescription("recording status", cron.recording)
	cron.SetDescription("startAttempted", cron.startAttempted)
	// #region agent log
	debugAgentLog("A", "crontab.go:startRecording/ok", "开始录制成功，挂上每10秒再试帮手", map[string]any{
		"streamPath": streamPath,
	})
	// #endregion
	cron.ensureRetryWatcher()
}

// 停止录制
func (cron *Crontab) stopRecording() {
	cron.recordMu.Lock()
	if !cron.recording {
		cron.recordMu.Unlock()
		return
	}

	now := time.Now().In(cron.location)
	cron.Info("crontab", "event", "stop recording", "plan", cron.RecordPlan.Name, "time", now)

	wasRecording := cron.recording
	cron.recording = false
	savedSlot := cron.currentSlot
	cron.currentSlot = nil
	cron.startAttempted = false
	retry := cron.retryTask
	cron.retryTask = nil
	recordType := cron.RecordType
	streamPath := cron.StreamPath
	cron.recordMu.Unlock()

	if retry != nil {
		retry.Stop(errors.New("stop recording"))
	}

	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	ctx, cancel := cron.httpContext(10 * time.Second)
	defer cancel()

	stopURL := fmt.Sprintf("http://%s/%s/api/stop/%s", addr, pluginAPIName(recordType), streamPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stopURL, nil)
	if err != nil {
		cron.Error("crontab", "err", "build stop request failed", "detail", err)
		cron.recordMu.Lock()
		if wasRecording && !cron.IsStopped() {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
		cron.recordMu.Unlock()
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cron.Error("crontab", "err", "stop recording failed", "detail", err)
		cron.recordMu.Lock()
		if wasRecording && !cron.IsStopped() {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
		cron.recordMu.Unlock()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cron.Error("crontab", "err", "stop recording failed", "status", resp.StatusCode)
		cron.recordMu.Lock()
		if wasRecording && !cron.IsStopped() {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
		cron.recordMu.Unlock()
	}
	cron.SetDescription("recording status", cron.recording)
	cron.SetDescription("startAttempted", cron.startAttempted)
}

// ensureRetryWatcher 确保「每 10 秒检查并补开录」的帮手已启动。
// 已有帮手或任务已停止时直接返回，因此可以在成功/失败/长睡前多处调用，不会起多个帮手。
func (cron *Crontab) ensureRetryWatcher() {
	if cron.retryTask != nil || cron.IsStopped() {
		// #region agent log
		debugAgentLog("C", "crontab.go:ensureRetryWatcher/skip", "帮手已存在或任务已停，跳过", map[string]any{
			"streamPath":   cron.StreamPath,
			"hasRetryTask": cron.retryTask != nil,
			"isStopped":    cron.IsStopped(),
		})
		// #endregion
		return
	}
	cron.retryTask = &RecordRetryTickTask{
		cron:     cron,
		interval: 10 * time.Second,
	}
	cron.retryTask.OnStop(func() {
		cron.retryTask = nil
	})
	// #region agent log
	debugAgentLog("C", "crontab.go:ensureRetryWatcher/create", "新建每10秒再试帮手", map[string]any{
		"streamPath":  cron.StreamPath,
		"intervalSec": 10,
	})
	// #endregion
	cron.AddTask(cron.retryTask)
}
