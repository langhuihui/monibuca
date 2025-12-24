package plugin_crontab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
type Crontab struct {
	task.Work
	ctp *CrontabPlugin
	*pkg.RecordPlan
	*pkg.RecordPlanStream

	stop           chan struct{}
	running        bool
	location       *time.Location
	timer          *time.Timer
	currentSlot    *TimeSlot // current slot
	recording      bool      // currently recording
	startAttempted bool      // startRecording already tried in this slot
	retryTask      *RecordRetryTickTask
}

func (cron *Crontab) GetKey() string {
	return strconv.Itoa(int(cron.PlanID)) + "_" + cron.StreamPath
}

// 初始化
func (cron *Crontab) Start() (err error) {
	cron.Info("crontab", "event", "plugin start")

	// 初始化必要字段
	if cron.stop == nil {
		cron.stop = make(chan struct{})
	}
	if cron.location == nil {
		cron.location = time.Local
	}

	cron.running = true

	cron.SetDescription("streampath", cron.StreamPath)
	cron.SetDescription("planid", cron.PlanID)
	cron.SetDescription("recording status", cron.recording)
	return nil
}

// 阻塞运行
func (cron *Crontab) Go() (err error) {
	cron.Info("crontab", "event", "plugin running")
	cron.Info("crontab", "event", "scheduler start")

	for {
		// get current time
		now := time.Now().In(cron.location)

		// immediate actions (e.g., stop)
		if cron.recording && cron.currentSlot != nil &&
			(now.Equal(cron.currentSlot.End) || now.After(cron.currentSlot.End)) {
			cron.stopRecording()
			continue
		}

		// determine next event
		var nextEvent time.Time

		if cron.recording {
			// when recording, next is end
			nextEvent = cron.currentSlot.End
		} else {
			// when idle, check if current slot is still valid
			var nextSlot *TimeSlot
			if cron.currentSlot != nil && now.After(cron.currentSlot.Start) && now.Before(cron.currentSlot.End) {
				// current slot is still valid, reuse it
				nextSlot = cron.currentSlot
				cron.Debug("crontab", "msg", "reuse current slot", "start", nextSlot.Start.Format("2006-01-02 15:04:05"), "end", nextSlot.End.Format("2006-01-02 15:04:05"))
				// ensure retryTask exists (may have been stopped for some reason)
				cron.ensureRetryWatcher()
			} else {
				// current slot expired or not exists, find next start
				nextSlot = cron.getNextTimeSlot()
				if nextSlot == nil {
					// no plan, wait 1h
					cron.timer = time.NewTimer(1 * time.Hour)
					cron.Info("crontab", "event", "no plan", "action", "wait 1h")

					// wait timer or stop
					select {
					case <-cron.timer.C:
						continue
					case <-cron.stop:
						// stop scheduler
						if cron.timer != nil {
							cron.timer.Stop()
						}
						cron.Info("crontab", "event", "scheduler stop")
						return
					}
				}

				// only update slot and restart retryTask when slot actually changed
				if cron.currentSlot == nil || !nextSlot.Start.Equal(cron.currentSlot.Start) || !nextSlot.End.Equal(cron.currentSlot.End) {
					cron.Info("crontab", "into cron.currentSlot == nil || !nextSlot.Start.Equal(cron.currentSlot.Start) || !nextSlot.End.Equal(cron.currentSlot.End)", "")
					cron.currentSlot = nextSlot
					// reset flags for new slot
					cron.startAttempted = false
					if cron.retryTask != nil {
						cron.retryTask.Stop(errors.New("switch time slot"))
						cron.retryTask = nil
					}
					//cron.ensureRetryWatcher()
				}
			}

			nextEvent = nextSlot.Start

			// if start already passed, start now
			if now.Equal(nextEvent) || now.After(nextEvent) {
				if !cron.startAttempted {
					cron.startRecording()
				} else {
					cron.Debug("crontab", "msg", "startRecording already attempted in this slot1")
				}
				continue
			}
		}

		// wait duration
		waitDuration := nextEvent.Sub(now)

		// negative wait => execute now
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

		// set timer
		timer := time.NewTimer(waitDuration)

		if !cron.recording {
			cron.Info("crontab", "next_start", nextEvent, "wait", waitDuration)
			cron.SetDescription("current step", "wait next start "+nextEvent.Format("2006-01-02 15:04:05"))
		} else {
			cron.Info("crontab", "next_end", nextEvent, "wait", waitDuration)
			cron.SetDescription("current step", "wait next stop "+nextEvent.Format("2006-01-02 15:04:05"))
		}

		// wait timer or stop
		select {
		case <-timer.C:
			// execute
			if !cron.recording {
				if !cron.startAttempted {
					cron.startRecording()
				} else {
					cron.Debug("crontab", "msg", "startRecording already attempted in this slot3")
				}
			} else {
				cron.stopRecording()
			}

		case <-cron.stop:
			// stop scheduler
			timer.Stop()
			cron.Info("crontab", "event", "scheduler stop")
			return
		}
	}
}

// 停止
func (cron *Crontab) Dispose() {
	//if cron.running {
	//cron.stop <- struct{}{}
	close(cron.stop) // 关闭通道会触发 <-cron.stop
	cron.running = false
	if cron.timer != nil {
		cron.timer.Stop()
	}
	if cron.retryTask != nil {
		cron.retryTask.Stop(errors.New("crontab disposed"))
		cron.retryTask = nil
	}

	// 如果还在录制，停止录制
	if cron.recording {
		cron.stopRecording()
	}
	//}
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

// 开始录制
func (cron *Crontab) startRecording() {
	cron.Debug("crontab", "startRecording recording", cron.recording)
	if cron.recording {
		return // already recording
	}

	// mark attempt in current slot

	cron.startAttempted = false
	cron.Debug("crontab", "before send record post,set cron.startAttempted", cron.startAttempted)
	now := time.Now().In(cron.location)
	cron.Info("crontab", "event", "start recording", "plan", cron.RecordPlan.Name, "time", now, "plan_end", cron.currentSlot.End)

	// 构造请求体
	reqBody := map[string]string{
		"fragment": cron.Fragment,
		"filePath": cron.FilePath,
		"mode":     "auto",
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		cron.Error("crontab", "err", "build request body failed", "detail", err)
		return
	}

	// resolve HTTP address
	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080" // default port
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	// 发送开始录制请求
	resp, err := http.Post(fmt.Sprintf("http://%s/mp4/api/start/%s", addr, cron.StreamPath), "application/json", bytes.NewBuffer(jsonBody))
	cron.Debug("crontab", "record_request_url", fmt.Sprintf("http://%s/mp4/api/start/%s", addr,
		cron.StreamPath), "body", string(jsonBody))
	if err != nil {
		time.Sleep(time.Second)
		cron.Error("crontab", "err", "start recording failed", "detail", err)
		return
	}
	defer resp.Body.Close()
	respJSON, _ := json.Marshal(resp.Body)
	cron.SetDescription("response.Body", string(respJSON))
	cron.SetDescription("response.StatusCode", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		time.Sleep(time.Second)
		cron.Error("crontab", "err", "start recording failed", "status", resp.StatusCode)
		return
	}
	cron.startAttempted = true
	cron.Debug("crontab", "set cron.startAttempted", cron.startAttempted)
	cron.recording = true
	cron.SetDescription("recording status", cron.recording)
	cron.SetDescription("startAttempted", cron.startAttempted)
	cron.ensureRetryWatcher()
}

// 停止录制
func (cron *Crontab) stopRecording() {
	cron.Debug("crontab", "stopRecording", "")
	if !cron.recording {
		return // not recording
	}

	// 立即记录当前时间并重置状态，避免重复调用
	now := time.Now().In(cron.location)
	cron.Info("crontab", "event", "stop recording", "plan", cron.RecordPlan.Name, "time", now)

	// 先重置状态，避免循环中重复检测到停止条件
	wasRecording := cron.recording
	cron.recording = false
	savedSlot := cron.currentSlot
	cron.currentSlot = nil
	cron.startAttempted = false
	if cron.retryTask != nil {
		cron.retryTask.Stop(errors.New("stop recording"))
		cron.retryTask = nil
	}

	// resolve HTTP address
	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080" // default port
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	// 发送停止录制请求
	resp, err := http.Post(fmt.Sprintf("http://%s/mp4/api/stop/%s", addr, cron.StreamPath), "application/json", nil)
	if err != nil {
		cron.Error("crontab", "err", "stop recording failed", "detail", err)
		// 如果请求失败，恢复状态以便下次重试
		if wasRecording {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cron.Error("crontab", "err", "stop recording failed", "status", resp.StatusCode)
		// 如果请求失败，恢复状态以便下次重试
		if wasRecording {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
	}
	cron.SetDescription("recording status", cron.recording)
	cron.SetDescription("startAttempted", cron.startAttempted)
}

// ensureRetryWatcher ensures retry task started
func (cron *Crontab) ensureRetryWatcher() {
	if cron.retryTask != nil {
		return
	}
	cron.retryTask = &RecordRetryTickTask{
		cron:     cron,
		interval: 10 * time.Second,
	}
	cron.retryTask.OnStop(func() {
		cron.retryTask = nil
	})
	// start as sub task of current cron to avoid cross-plugin registration
	cron.AddTask(cron.retryTask)
}
