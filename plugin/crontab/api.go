package plugin_crontab

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	cronpb "m7s.live/v5/plugin/crontab/pb"
	"m7s.live/v5/plugin/crontab/pkg"
)

func (ct *CrontabPlugin) List(ctx context.Context, req *cronpb.ReqPlanList) (*cronpb.PlanResponseList, error) {
	if req.PageNum < 1 {
		req.PageNum = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 10
	}

	// 从内存中获取所有计划
	plans := ct.recordPlans.Items
	total := len(plans)

	// 计算分页
	start := int(req.PageNum-1) * int(req.PageSize)
	end := start + int(req.PageSize)
	if start >= total {
		start = total
	}
	if end > total {
		end = total
	}

	// 获取当前页的数据
	pagePlans := plans[start:end]

	data := make([]*cronpb.Plan, 0, len(pagePlans))
	for _, plan := range pagePlans {
		data = append(data, &cronpb.Plan{
			Id:         uint32(plan.ID),
			Name:       plan.Name,
			Enable:     plan.Enable,
			CreateTime: timestamppb.New(plan.CreatedAt),
			UpdateTime: timestamppb.New(plan.UpdatedAt),
			Plan:       plan.Plan,
		})
	}

	return &cronpb.PlanResponseList{
		Code:       0,
		Message:    "success",
		TotalCount: uint32(total),
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		Data:       data,
	}, nil
}

func (ct *CrontabPlugin) Add(ctx context.Context, req *cronpb.Plan) (*cronpb.Response, error) {
	// 参数验证
	if strings.TrimSpace(req.Name) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "name is required",
		}, nil
	}

	if strings.TrimSpace(req.Plan) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "plan is required",
		}, nil
	}

	// 检查名称是否已存在
	var count int64
	if err := ct.DB.Model(&pkg.RecordPlan{}).Where("name = ?", req.Name).Count(&count).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	if count > 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "name already exists",
		}, nil
	}

	plan := &pkg.RecordPlan{
		Name:   req.Name,
		Plan:   req.Plan,
		Enable: req.Enable,
	}

	if err := ct.DB.Create(plan).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 添加到内存中
	ct.recordPlans.Add(plan)

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

func (ct *CrontabPlugin) Update(ctx context.Context, req *cronpb.Plan) (*cronpb.Response, error) {
	if req.Id == 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "id is required",
		}, nil
	}

	// 参数验证
	if strings.TrimSpace(req.Name) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "name is required",
		}, nil
	}

	if strings.TrimSpace(req.Plan) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "plan is required",
		}, nil
	}

	// 检查记录是否存在
	var existingPlan pkg.RecordPlan
	if err := ct.DB.First(&existingPlan, req.Id).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record not found",
		}, nil
	}

	// 检查新名称是否与其他记录冲突
	var count int64
	if err := ct.DB.Model(&pkg.RecordPlan{}).Where("name = ? AND id != ?", req.Name, req.Id).Count(&count).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	if count > 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "name already exists",
		}, nil
	}

	// 处理 enable 状态变更
	enableChanged := existingPlan.Enable != req.Enable

	// 更新记录
	updates := map[string]interface{}{
		"name":   req.Name,
		"plan":   req.Plan,
		"enable": req.Enable,
	}

	if err := ct.DB.Model(&existingPlan).Updates(updates).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 更新内存中的记录
	existingPlan.Name = req.Name
	existingPlan.Plan = req.Plan
	existingPlan.Enable = req.Enable
	ct.recordPlans.Set(&existingPlan)

	// 处理 enable 状态变更后的操作
	if enableChanged {
		if req.Enable {
			// 从 false 变为 true，需要创建并启动新的定时任务
			var streams []pkg.RecordPlanStream
			model := &pkg.RecordPlanStream{PlanID: existingPlan.ID}
			if err := ct.DB.Model(model).Where(model).Find(&streams).Error; err != nil {
				ct.Error("query record plan streams error: %v", err)
			} else {
				// 为每个流创建定时任务
				for _, stream := range streams {
					crontab := &Crontab{
						ctp:              ct,
						RecordPlan:       &existingPlan,
						RecordPlanStream: &stream,
					}
					crontab.Logger = ct.Logger.With("streamPath", crontab.StreamPath)
					//crontab.OnStart(func() {
					//	ct.crontabs.Set(crontab)
					//})
					ct.crontabs.AddTask(crontab)
				}
			}
		} else {
			// 从 true 变为 false，需要停止相关的定时任务
			ct.crontabs.Range(func(crontab *Crontab) bool {
				if crontab.RecordPlan.ID == existingPlan.ID {
					crontab.Stop(errors.New("plan disabled"))
				}
				return true
			})
		}
	}

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

func (ct *CrontabPlugin) Remove(ctx context.Context, req *cronpb.DeleteRequest) (*cronpb.Response, error) {
	if req.Id == 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "id is required",
		}, nil
	}

	// 检查记录是否存在
	var existingPlan pkg.RecordPlan
	if err := ct.DB.First(&existingPlan, req.Id).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record not found",
		}, nil
	}

	// 先停止所有相关的定时任务
	ct.crontabs.Range(func(crontab *Crontab) bool {
		if crontab.RecordPlan.ID == existingPlan.ID {
			crontab.Stop(errors.New("plan stream removed"))
		}
		return true
	})

	// 执行软删除
	if err := ct.DB.Delete(&existingPlan).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 从内存中移除
	ct.recordPlans.RemoveByKey(existingPlan.ID)

	if err := ct.DB.Where("plan_id = ?", req.Id).Delete(&pkg.RecordPlanStream{}).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

func (ct *CrontabPlugin) ListRecordPlanStreams(ctx context.Context, req *cronpb.ReqPlanStreamList) (*cronpb.RecordPlanStreamResponseList, error) {
	if req.PageNum < 1 {
		req.PageNum = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 10
	}

	var total int64
	var streams []pkg.RecordPlanStream
	model := &pkg.RecordPlanStream{}

	// 构建查询条件
	query := ct.DB.Model(model).
		Scopes(
			pkg.ScopeRecordPlanID(uint(req.PlanId)),
			pkg.ScopeStreamPathLike(req.StreamPath),
			pkg.ScopeOrderByCreatedAtDesc(),
		)

	result := query.Count(&total)
	if result.Error != nil {
		return &cronpb.RecordPlanStreamResponseList{
			Code:    500,
			Message: result.Error.Error(),
		}, nil
	}

	offset := (req.PageNum - 1) * req.PageSize
	result = query.Offset(int(offset)).Limit(int(req.PageSize)).Find(&streams)

	if result.Error != nil {
		return &cronpb.RecordPlanStreamResponseList{
			Code:    500,
			Message: result.Error.Error(),
		}, nil
	}

	data := make([]*cronpb.PlanStream, 0, len(streams))
	for _, stream := range streams {
		data = append(data, &cronpb.PlanStream{
			PlanId:     uint32(stream.PlanID),
			StreamPath: stream.StreamPath,
			Fragment:   stream.Fragment,
			FilePath:   stream.FilePath,
			CreatedAt:  timestamppb.New(stream.CreatedAt),
			UpdatedAt:  timestamppb.New(stream.UpdatedAt),
			Enable:     stream.Enable,
		})
	}

	return &cronpb.RecordPlanStreamResponseList{
		Code:       0,
		Message:    "success",
		TotalCount: uint32(total),
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		Data:       data,
	}, nil
}

func (ct *CrontabPlugin) AddRecordPlanStream(ctx context.Context, req *cronpb.PlanStream) (*cronpb.Response, error) {
	planId := 1
	if req.PlanId > 0 {
		planId = int(req.PlanId)
	}
	recordType := "mp4"
	if req.RecordType != "" {
		recordType = req.RecordType
	}

	if strings.TrimSpace(req.StreamPath) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "stream_path is required",
		}, nil
	}

	// 从内存中获取录制计划
	plan, ok := ct.recordPlans.Get(uint(planId))
	if !ok {
		return &cronpb.Response{
			Code:    404,
			Message: "record plan not found",
		}, nil
	}

	// 检查是否已存在相同的记录
	var count int64
	searchModel := pkg.RecordPlanStream{
		PlanID:     uint(planId),
		StreamPath: req.StreamPath,
		RecordType: recordType,
	}
	if err := ct.DB.Model(&searchModel).Where(&searchModel).Count(&count).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	if count > 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "record already exists",
		}, nil
	}

	fragment := "60s"

	if req.Fragment != "" {
		fragment = req.Fragment
	}

	stream := &pkg.RecordPlanStream{
		PlanID:     uint(planId),
		StreamPath: req.StreamPath,
		Fragment:   fragment,
		FilePath:   req.FilePath,
		Enable:     req.Enable,
		RecordType: req.RecordType,
	}

	if err := ct.DB.Create(stream).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 如果计划是启用状态，创建并启动定时任务
	if plan.Enable {
		crontab := &Crontab{
			ctp:              ct,
			RecordPlan:       plan,
			RecordPlanStream: stream,
		}
		crontab.Logger = ct.Logger.With("streamPath", crontab.StreamPath)
		//crontab.OnStart(func() {
		//	ct.crontabs.Set(crontab)
		//})
		ct.crontabs.AddTask(crontab)
	}

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

func (ct *CrontabPlugin) UpdateRecordPlanStream(ctx context.Context, req *cronpb.PlanStream) (*cronpb.Response, error) {
	planId := 1
	if req.PlanId > 0 {
		planId = int(req.PlanId)
	}

	if strings.TrimSpace(req.StreamPath) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "stream_path is required",
		}, nil
	}

	if strings.TrimSpace(req.RecordType) == "" {
		req.RecordType = "mp4"
	}

	// 检查记录是否存在
	var existingStream pkg.RecordPlanStream
	searchModel := pkg.RecordPlanStream{
		PlanID:     uint(planId),
		StreamPath: req.StreamPath,
		RecordType: req.RecordType,
	}
	if err := ct.DB.Where(&searchModel).First(&existingStream).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record not found",
		}, nil
	}

	// 更新记录
	existingStream.Fragment = req.Fragment
	existingStream.FilePath = req.FilePath
	existingStream.Enable = req.Enable
	existingStream.RecordType = req.RecordType

	if err := ct.DB.Save(&existingStream).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 停止当前流相关的所有任务
	ct.crontabs.Range(func(crontab *Crontab) bool {
		if crontab.RecordPlanStream.StreamPath == req.StreamPath && crontab.RecordType == req.RecordType && crontab.PlanID == uint(planId) {
			crontab.Stop(errors.New("record plan changed"))
		}
		return true
	})

	// 查询所有关联此流的记录
	var streams []pkg.RecordPlanStream
	if err := ct.DB.Where(&pkg.RecordPlanStream{
		PlanID:     uint(req.PlanId),
		StreamPath: req.StreamPath,
		RecordType: req.RecordType,
	}).Find(&streams).Error; err != nil {
		ct.Error("query record plan streams error: %v", err)
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 为每个启用的计划创建新的定时任务
	for _, stream := range streams {
		// 从内存中获取对应的计划
		plan, ok := ct.recordPlans.Get(stream.PlanID)
		if !ok {
			ct.Error("record plan not found in memory: %d", stream.PlanID)
			continue
		}

		// 如果计划是启用状态，创建并启动定时任务
		if plan.Enable && stream.Enable {
			crontab := &Crontab{
				ctp:              ct,
				RecordPlan:       plan,
				RecordPlanStream: &stream,
			}
			crontab.Logger = ct.Logger.With("streamPath", crontab.StreamPath)
			//crontab.OnStart(func() {
			//	ct.crontabs.Set(crontab)
			//})
			ct.crontabs.AddTask(crontab)
		}
	}

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

func (ct *CrontabPlugin) RemoveRecordPlanStream(ctx context.Context, req *cronpb.DeletePlanStreamRequest) (*cronpb.Response, error) {
	if req.PlanId == 0 {
		return &cronpb.Response{
			Code:    400,
			Message: "record_plan_id is required",
		}, nil
	}

	if strings.TrimSpace(req.StreamPath) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "stream_path is required",
		}, nil
	}

	if strings.TrimSpace(req.RecordType) == "" {
		return &cronpb.Response{
			Code:    400,
			Message: "recordType is required",
		}, nil
	}

	// 检查记录是否存在
	var existingStream pkg.RecordPlanStream
	searchModel := pkg.RecordPlanStream{
		PlanID:     uint(req.PlanId),
		StreamPath: req.StreamPath,
		RecordType: req.RecordType,
	}
	if err := ct.DB.Where(&searchModel).First(&existingStream).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record not found",
		}, nil
	}

	// 停止所有相关的定时任务
	ct.crontabs.Range(func(crontab *Crontab) bool {
		if crontab.RecordPlanStream.StreamPath == req.StreamPath && crontab.RecordPlan.ID == uint(req.PlanId) && crontab.RecordPlanStream.RecordType == req.RecordType {
			crontab.Stop(errors.New("remove record plan"))
		}
		return true
	})

	// 执行删除
	if err := ct.DB.Delete(&existingStream).Error; err != nil {
		return &cronpb.Response{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	return &cronpb.Response{
		Code:    0,
		Message: "success",
	}, nil
}

// 获取周几的名称（0=周日，1=周一，...，6=周六）
func getWeekdayName(weekday int) string {
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	return weekdays[weekday]
}

// 获取周几的索引（0=周日，1=周一，...，6=周六）
func getWeekdayIndex(weekdayName string) int {
	weekdays := map[string]int{
		"周日": 0, "周一": 1, "周二": 2, "周三": 3, "周四": 4, "周五": 5, "周六": 6,
	}
	return weekdays[weekdayName]
}

// 获取下一个指定周几的日期
func getNextDateForWeekday(now time.Time, targetWeekday int, location *time.Location) time.Time {
	nowWeekday := int(now.Weekday())
	daysToAdd := 0

	if targetWeekday >= nowWeekday {
		daysToAdd = targetWeekday - nowWeekday
	} else {
		daysToAdd = 7 - (nowWeekday - targetWeekday)
	}

	// 如果是同一天但当前时间已经过了最后的时间段，则推到下一周
	if daysToAdd == 0 {
		// 这里简化处理，直接加7天到下周同一天
		daysToAdd = 7
	}

	return now.AddDate(0, 0, daysToAdd)
}

// 计算计划中的所有时间段
func calculateTimeSlots(plan string, now time.Time, location *time.Location) ([]*cronpb.TimeSlotInfo, error) {
	if len(plan) != 168 {
		return nil, fmt.Errorf("invalid plan format: length should be 168")
	}

	var slots []*cronpb.TimeSlotInfo

	// 按周几遍历（0=周日，1=周一，...，6=周六）
	for weekday := 0; weekday < 7; weekday++ {
		dayOffset := weekday * 24
		var startHour int = -1

		// 遍历这一天的每个小时
		for hour := 0; hour <= 24; hour++ {
			// 如果到了一天的结尾或者当前小时状态为0
			isEndOfDay := hour == 24
			isHourOff := !isEndOfDay && plan[dayOffset+hour] == '0'

			if isEndOfDay || isHourOff {
				// 如果之前有开始的时间段，现在结束了
				if startHour != -1 {
					// 计算下一个该周几的日期
					targetDate := getNextDateForWeekday(now, weekday, location)

					// 创建时间段
					startTime := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), startHour, 0, 0, 0, location)
					endTime := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), hour, 0, 0, 0, location)

					// 转换为 UTC 时间
					startTs := timestamppb.New(startTime.UTC())
					endTs := timestamppb.New(endTime.UTC())

					slots = append(slots, &cronpb.TimeSlotInfo{
						Start:     startTs,
						End:       endTs,
						Weekday:   getWeekdayName(weekday),
						TimeRange: fmt.Sprintf("%02d:00-%02d:00", startHour, hour),
					})
					startHour = -1
				}
			} else if plan[dayOffset+hour] == '1' && startHour == -1 {
				// 找到新的开始时间
				startHour = hour
			}
		}
	}

	// 按时间排序
	sort.Slice(slots, func(i, j int) bool {
		// 先按周几排序
		weekdayI := getWeekdayIndex(slots[i].Weekday)
		weekdayJ := getWeekdayIndex(slots[j].Weekday)

		if weekdayI != weekdayJ {
			return weekdayI < weekdayJ
		}

		// 同一天按开始时间排序
		return slots[i].Start.AsTime().Hour() < slots[j].Start.AsTime().Hour()
	})

	return slots, nil
}

// 获取下一个时间段（API 使用版），内部复用 crontab 的 nextSlotRange 逻辑。
// 这样 ParsePlanTime 和实际调度看到的“下一执行时间”是一致的。
func getNextTimeSlotFromNow(plan string, now time.Time, location *time.Location) (*cronpb.TimeSlotInfo, error) {
	if len(plan) != 168 {
		return nil, fmt.Errorf("invalid plan format: length should be 168")
	}

	start, end, ok := nextSlotRange(plan, now, location)
	if !ok {
		return nil, nil
	}

	// Weekday 与 TimeRange 使用本地时间表示，更贴近用户理解
	localStart := start.In(location)
	localEnd := end.In(location)
	weekday := int(localStart.Weekday())

	return &cronpb.TimeSlotInfo{
		Start:     timestamppb.New(start.UTC()),
		End:       timestamppb.New(end.UTC()),
		Weekday:   getWeekdayName(weekday),
		TimeRange: fmt.Sprintf("%02d:00-%02d:00", localStart.Hour(), localEnd.Hour()),
	}, nil
}

func (ct *CrontabPlugin) ParsePlanTime(ctx context.Context, req *cronpb.ParsePlanRequest) (*cronpb.ParsePlanResponse, error) {
	if len(req.Plan) != 168 {
		return &cronpb.ParsePlanResponse{
			Code:    400,
			Message: "invalid plan format: length should be 168",
		}, nil
	}

	// 检查字符串格式是否正确（只包含0和1）
	for i, c := range req.Plan {
		if c != '0' && c != '1' {
			return &cronpb.ParsePlanResponse{
				Code:    400,
				Message: fmt.Sprintf("invalid character at position %d: %c (should be 0 or 1)", i, c),
			}, nil
		}
	}

	// 获取所有时间段
	slots, err := calculateTimeSlots(req.Plan, time.Now(), time.Local)
	if err != nil {
		return &cronpb.ParsePlanResponse{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	// 获取下一个时间段
	nextSlot, err := getNextTimeSlotFromNow(req.Plan, time.Now(), time.Local)
	if err != nil {
		return &cronpb.ParsePlanResponse{
			Code:    500,
			Message: err.Error(),
		}, nil
	}

	return &cronpb.ParsePlanResponse{
		Code:     0,
		Message:  "success",
		Slots:    slots,
		NextSlot: nextSlot,
	}, nil
}

// 辅助函数：构建任务状态信息
func buildCrontabTaskInfo(crontab *Crontab, now time.Time) *cronpb.CrontabTaskInfo {
	// 基础任务信息
	taskInfo := &cronpb.CrontabTaskInfo{
		PlanId:     uint32(crontab.RecordPlan.ID),
		PlanName:   crontab.RecordPlan.Name,
		StreamPath: crontab.StreamPath,
		FilePath:   crontab.FilePath,
		Fragment:   crontab.Fragment,
	}

	// 获取完整计划时间段列表
	if crontab.RecordPlan != nil && crontab.RecordPlan.Plan != "" {
		planSlots, err := calculateTimeSlots(crontab.RecordPlan.Plan, now, time.Local)
		if err == nil && planSlots != nil && len(planSlots) > 0 {
			taskInfo.PlanSlots = planSlots
		}
	}

	return taskInfo
}

// GetCrontabStatus 获取当前Crontab任务状态
func (ct *CrontabPlugin) GetCrontabStatus(ctx context.Context, req *cronpb.CrontabStatusRequest) (*cronpb.CrontabStatusResponse, error) {
	response := &cronpb.CrontabStatusResponse{
		Code:         0,
		Message:      "success",
		RunningTasks: []*cronpb.CrontabTaskInfo{},
		NextTasks:    []*cronpb.CrontabTaskInfo{},
		TotalRunning: 0,
		TotalPlanned: 0,
	}

	// 获取当前正在运行的任务
	runningTasks := make([]*cronpb.CrontabTaskInfo, 0)
	nextTasks := make([]*cronpb.CrontabTaskInfo, 0)

	// 如果只指定了流路径但未找到对应的任务，也返回该流的计划信息
	streamPathFound := false

	// 遍历所有Crontab任务
	ct.crontabs.Range(func(crontab *Crontab) bool {
		// 如果指定了stream_path过滤条件，且不匹配，则跳过
		if req.StreamPath != "" && crontab.StreamPath != req.StreamPath {
			return true // 继续遍历
		}

		// 标记已找到指定的流
		if req.StreamPath != "" {
			streamPathFound = true
		}

		now := time.Now()

		// 构建基本任务信息
		taskInfo := buildCrontabTaskInfo(crontab, now)

		// 检查是否正在录制
		if crontab.recording && crontab.currentSlot != nil {
			// 当前正在录制
			taskInfo.IsRecording = true

			// 设置时间信息
			taskInfo.StartTime = timestamppb.New(crontab.currentSlot.Start)
			taskInfo.EndTime = timestamppb.New(crontab.currentSlot.End)

			// 计算已运行时间和剩余时间
			elapsedDuration := now.Sub(crontab.currentSlot.Start)
			remainingDuration := crontab.currentSlot.End.Sub(now)
			taskInfo.ElapsedSeconds = uint32(elapsedDuration.Seconds())
			taskInfo.RemainingSeconds = uint32(remainingDuration.Seconds())

			// 设置时间范围和周几
			startHour := crontab.currentSlot.Start.Hour()
			endHour := crontab.currentSlot.End.Hour()
			taskInfo.TimeRange = fmt.Sprintf("%02d:00-%02d:00", startHour, endHour)
			taskInfo.Weekday = getWeekdayName(int(crontab.currentSlot.Start.Weekday()))

			// 添加到正在运行的任务列表
			runningTasks = append(runningTasks, taskInfo)
		} else {
			// 获取下一个时间段
			nextSlot := crontab.getNextTimeSlot()
			if nextSlot != nil {
				// 设置下一个任务的信息
				taskInfo.IsRecording = false

				// 设置时间信息
				taskInfo.StartTime = timestamppb.New(nextSlot.Start)
				taskInfo.EndTime = timestamppb.New(nextSlot.End)

				// 计算等待时间
				waitingDuration := nextSlot.Start.Sub(now)
				taskInfo.RemainingSeconds = uint32(waitingDuration.Seconds())

				// 设置时间范围和周几
				startHour := nextSlot.Start.Hour()
				endHour := nextSlot.End.Hour()
				taskInfo.TimeRange = fmt.Sprintf("%02d:00-%02d:00", startHour, endHour)
				taskInfo.Weekday = getWeekdayName(int(nextSlot.Start.Weekday()))

				// 添加到计划任务列表
				nextTasks = append(nextTasks, taskInfo)
			}
		}

		return true // 继续遍历
	})

	// 如果指定了流路径但未找到对应的任务，查询数据库获取该流的计划信息
	if req.StreamPath != "" && !streamPathFound {
		// 查询与该流相关的所有计划
		var streams []pkg.RecordPlanStream
		if err := ct.DB.Where("stream_path = ?", req.StreamPath).Find(&streams).Error; err == nil && len(streams) > 0 {
			for _, stream := range streams {
				// 获取对应的计划
				var plan pkg.RecordPlan
				if err := ct.DB.First(&plan, stream.PlanID).Error; err == nil && plan.Enable && stream.Enable {
					now := time.Now()

					// 构建任务信息
					taskInfo := &cronpb.CrontabTaskInfo{
						PlanId:      uint32(plan.ID),
						PlanName:    plan.Name,
						StreamPath:  stream.StreamPath,
						FilePath:    stream.FilePath,
						Fragment:    stream.Fragment,
						IsRecording: false,
					}

					// 获取完整计划时间段列表
					planSlots, err := calculateTimeSlots(plan.Plan, now, time.Local)
					if err == nil && planSlots != nil && len(planSlots) > 0 {
						taskInfo.PlanSlots = planSlots
					}

					// 获取下一个时间段
					nextSlot, err := getNextTimeSlotFromNow(plan.Plan, now, time.Local)
					if err == nil && nextSlot != nil {
						// 设置时间信息
						taskInfo.StartTime = nextSlot.Start
						taskInfo.EndTime = nextSlot.End
						taskInfo.TimeRange = nextSlot.TimeRange
						taskInfo.Weekday = nextSlot.Weekday

						// 计算等待时间
						waitingDuration := nextSlot.Start.AsTime().Sub(now)
						taskInfo.RemainingSeconds = uint32(waitingDuration.Seconds())

						// 添加到计划任务列表
						nextTasks = append(nextTasks, taskInfo)
					}
				}
			}
		}
	}

	// 按开始时间排序下一个任务列表
	sort.Slice(nextTasks, func(i, j int) bool {
		return nextTasks[i].StartTime.AsTime().Before(nextTasks[j].StartTime.AsTime())
	})

	// 设置响应结果
	response.RunningTasks = runningTasks
	response.NextTasks = nextTasks
	response.TotalRunning = uint32(len(runningTasks))
	response.TotalPlanned = uint32(len(nextTasks))

	return response, nil
}
