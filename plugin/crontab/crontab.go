package plugin_crontab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/crontab/pkg"
)

// 计划时间段
type TimeSlot struct {
	Start time.Time // 开始时间
	End   time.Time // 结束时间
}

// Crontab 定时任务调度器
type Crontab struct {
	task.Job
	ctp *CrontabPlugin
	*pkg.RecordPlan
	*pkg.RecordPlanStream

	stop        chan struct{}
	running     bool
	location    *time.Location
	timer       *time.Timer
	currentSlot *TimeSlot // 当前执行的时间段
	recording   bool      // 是否正在录制
}

func (cron *Crontab) GetKey() string {
	return strconv.Itoa(int(cron.PlanID)) + "_" + cron.StreamPath
}

// 初始化
func (cron *Crontab) Start() (err error) {
	cron.Info("crontab plugin start")
	if cron.running {
		return // 已经运行中，不重复启动
	}

	// 初始化必要字段
	if cron.stop == nil {
		cron.stop = make(chan struct{})
	}
	if cron.location == nil {
		cron.location = time.Local
	}

	cron.running = true

	return nil
}

// 阻塞运行
func (cron *Crontab) Run() (err error) {
	cron.Info("crontab plugin is running")
	// 初始化必要字段
	if cron.stop == nil {
		cron.stop = make(chan struct{})
	}
	if cron.location == nil {
		cron.location = time.Local
	}

	cron.Info("调度器启动")

	for {
		// 获取当前时间
		now := time.Now().In(cron.location)

		// 首先检查是否需要立即执行操作（如停止录制）
		if cron.recording && cron.currentSlot != nil &&
			(now.Equal(cron.currentSlot.End) || now.After(cron.currentSlot.End)) {
			cron.stopRecording()
			continue
		}

		// 确定下一个事件
		var nextEvent time.Time
		var isStartEvent bool

		if cron.recording {
			// 如果正在录制，下一个事件是结束时间
			nextEvent = cron.currentSlot.End
			isStartEvent = false
		} else {
			// 如果没有录制，计算下一个开始时间
			nextSlot := cron.getNextTimeSlot()
			if nextSlot == nil {
				// 无法确定下次执行时间，使用默认间隔
				cron.timer = time.NewTimer(1 * time.Hour)
				cron.Info("无有效计划，等待1小时后重试")

				// 等待定时器或停止信号
				select {
				case <-cron.timer.C:
					continue // 继续循环
				case <-cron.stop:
					// 停止调度器
					if cron.timer != nil {
						cron.timer.Stop()
					}
					cron.Info("调度器停止")
					return
				}
			}

			cron.currentSlot = nextSlot
			nextEvent = nextSlot.Start
			isStartEvent = true

			// 如果已过开始时间，立即开始录制
			if now.Equal(nextEvent) || now.After(nextEvent) {
				cron.startRecording()
				continue
			}
		}

		// 计算等待时间
		waitDuration := nextEvent.Sub(now)

		// 如果等待时间为负，立即执行
		if waitDuration <= 0 {
			if isStartEvent {
				cron.startRecording()
			} else {
				cron.stopRecording()
			}
			continue
		}

		// 设置定时器
		timer := time.NewTimer(waitDuration)

		if isStartEvent {
			cron.Info("下次开始时间: ", nextEvent, "等待时间:", waitDuration)
		} else {
			cron.Info("下次结束时间: ", nextEvent, " 等待时间:", waitDuration)
		}

		// 等待定时器或停止信号
		select {
		case now = <-timer.C:
			// 更新当前时间为定时器触发时间
			now = now.In(cron.location)

			// 执行任务
			if isStartEvent {
				cron.startRecording()
			} else {
				cron.stopRecording()
			}

		case <-cron.stop:
			// 停止调度器
			timer.Stop()
			cron.Info("调度器停止")
			return
		}
	}
}

// 停止
func (cron *Crontab) Dispose() (err error) {
	if cron.running {
		cron.stop <- struct{}{}
		cron.running = false
		if cron.timer != nil {
			cron.timer.Stop()
		}

		// 如果还在录制，停止录制
		if cron.recording {
			cron.stopRecording()
		}
	}
	return
}

// 获取下一个时间段
func (cron *Crontab) getNextTimeSlot() *TimeSlot {
	if cron.RecordPlan == nil || !cron.RecordPlan.Enable || cron.RecordPlan.Plan == "" {
		return nil // 无有效计划
	}

	plan := cron.RecordPlan.Plan
	if len(plan) != 168 {
		cron.Error("无效的计划格式: %s, 长度应为168", plan)
		return nil
	}

	// 使用当地时间
	now := time.Now().In(cron.location)
	cron.Debug("当前本地时间: %v, 星期%d, 小时%d", now.Format("2006-01-02 15:04:05"), now.Weekday(), now.Hour())

	// 当前小时
	currentWeekday := int(now.Weekday())
	currentHour := now.Hour()

	// 检查是否在整点边界附近(前后30秒)
	isNearHourBoundary := now.Minute() == 59 && now.Second() >= 30 || now.Minute() == 0 && now.Second() <= 30

	// 首先检查当前时间是否在某个时间段内
	dayOffset := currentWeekday * 24
	if currentHour < 24 && plan[dayOffset+currentHour] == '1' {
		// 找到当前小时所在的完整时间段
		startHour := currentHour
		// 向前查找时间段的开始
		for h := currentHour - 1; h >= 0; h-- {
			if plan[dayOffset+h] == '1' {
				startHour = h
			} else {
				break
			}
		}

		// 向后查找时间段的结束
		endHour := currentHour + 1
		for h := endHour; h < 24; h++ {
			if plan[dayOffset+h] == '1' {
				endHour = h + 1
			} else {
				break
			}
		}

		// 检查我们是否已经接近当前时间段的结束
		isNearEndOfTimeSlot := currentHour == endHour-1 && now.Minute() == 59 && now.Second() >= 30

		// 如果我们靠近时间段结束且在小时边界附近，我们跳过此时间段，找下一个
		if isNearEndOfTimeSlot && isNearHourBoundary {
			cron.Debug("接近当前时间段结束，准备查找下一个时间段")
		} else {
			// 创建时间段
			startTime := time.Date(now.Year(), now.Month(), now.Day(), startHour, 0, 0, 0, cron.location)
			endTime := time.Date(now.Year(), now.Month(), now.Day(), endHour, 0, 0, 0, cron.location)

			// 如果当前时间已经接近或超过了结束时间，调整结束时间
			if now.After(endTime.Add(-30*time.Second)) || now.Equal(endTime) {
				cron.Debug("当前时间已接近或超过结束时间，尝试查找下一个时间段")
			} else {
				cron.Debug("当前已在有效时间段内: 开始=%v, 结束=%v",
					startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))

				return &TimeSlot{
					Start: startTime,
					End:   endTime,
				}
			}
		}
	}

	// 查找下一个时间段
	// 先查找当天剩余时间
	for h := currentHour + 1; h < 24; h++ {
		if plan[dayOffset+h] == '1' {
			// 找到开始小时
			startHour := h
			// 查找结束小时
			endHour := h + 1
			for j := h + 1; j < 24; j++ {
				if plan[dayOffset+j] == '1' {
					endHour = j + 1
				} else {
					break
				}
			}

			// 创建时间段
			startTime := time.Date(now.Year(), now.Month(), now.Day(), startHour, 0, 0, 0, cron.location)
			endTime := time.Date(now.Year(), now.Month(), now.Day(), endHour, 0, 0, 0, cron.location)

			cron.Debug("找到今天的有效时间段: 开始=%v, 结束=%v",
				startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))

			return &TimeSlot{
				Start: startTime,
				End:   endTime,
			}
		}
	}

	// 如果当天没有找到，则查找后续日期
	for d := 1; d <= 7; d++ {
		nextDay := (currentWeekday + d) % 7
		dayOffset := nextDay * 24

		for h := 0; h < 24; h++ {
			if plan[dayOffset+h] == '1' {
				// 找到开始小时
				startHour := h
				// 查找结束小时
				endHour := h + 1
				for j := h + 1; j < 24; j++ {
					if plan[dayOffset+j] == '1' {
						endHour = j + 1
					} else {
						break
					}
				}

				// 计算日期
				nextDate := now.AddDate(0, 0, d)

				// 创建时间段
				startTime := time.Date(nextDate.Year(), nextDate.Month(), nextDate.Day(), startHour, 0, 0, 0, cron.location)
				endTime := time.Date(nextDate.Year(), nextDate.Month(), nextDate.Day(), endHour, 0, 0, 0, cron.location)

				cron.Debug("找到未来有效时间段: 开始=%v, 结束=%v",
					startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))

				return &TimeSlot{
					Start: startTime,
					End:   endTime,
				}
			}
		}
	}

	cron.Debug("未找到有效的时间段")
	return nil
}

// 开始录制
func (cron *Crontab) startRecording() {
	if cron.recording {
		return // 已经在录制了
	}

	now := time.Now().In(cron.location)
	cron.Info("开始录制任务: %s, 时间: %v, 计划结束时间: %v",
		cron.RecordPlan.Name, now, cron.currentSlot.End)

	// 构造请求体
	reqBody := map[string]string{
		"fragment": cron.Fragment,
		"filePath": cron.FilePath,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		cron.Error("构造请求体失败: %v", err)
		return
	}

	// 获取 HTTP 地址
	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080" // 使用默认端口
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	// 发送开始录制请求
	resp, err := http.Post(fmt.Sprintf("http://%s/mp4/api/start/%s", addr, cron.StreamPath), "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		cron.Error("开始录制失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cron.Error("开始录制失败，HTTP状态码: %d", resp.StatusCode)
		return
	}

	cron.recording = true
}

// 停止录制
func (cron *Crontab) stopRecording() {
	if !cron.recording {
		return // 没有在录制
	}

	// 立即记录当前时间并重置状态，避免重复调用
	now := time.Now().In(cron.location)
	cron.Info("停止录制任务: %s, 时间: %v", cron.RecordPlan.Name, now)

	// 先重置状态，避免循环中重复检测到停止条件
	wasRecording := cron.recording
	cron.recording = false
	savedSlot := cron.currentSlot
	cron.currentSlot = nil

	// 获取 HTTP 地址
	addr := cron.ctp.Plugin.GetCommonConf().HTTP.ListenAddr
	if addr == "" {
		addr = ":8080" // 使用默认端口
	}
	if addr[0] == ':' {
		addr = "localhost" + addr
	}

	// 发送停止录制请求
	resp, err := http.Post(fmt.Sprintf("http://%s/mp4/api/stop/%s", addr, cron.StreamPath), "application/json", nil)
	if err != nil {
		cron.Error("停止录制失败: %v", err)
		// 如果请求失败，恢复状态以便下次重试
		if wasRecording {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cron.Error("停止录制失败，HTTP状态码: %d", resp.StatusCode)
		// 如果请求失败，恢复状态以便下次重试
		if wasRecording {
			cron.recording = true
			cron.currentSlot = savedSlot
		}
	}
}
