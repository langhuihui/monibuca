package plugin_crontab

import (
	"context"
	"strings"

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

	var total int64
	var plans []pkg.RecordPlan

	query := ct.DB.Model(&pkg.RecordPlan{})

	result := query.Count(&total)
	if result.Error != nil {
		return &cronpb.PlanResponseList{
			Code:    500,
			Message: result.Error.Error(),
		}, nil
	}

	offset := (req.PageNum - 1) * req.PageSize
	result = query.Order("id desc").Offset(int(offset)).Limit(int(req.PageSize)).Find(&plans)
	if result.Error != nil {
		return &cronpb.PlanResponseList{
			Code:    500,
			Message: result.Error.Error(),
		}, nil
	}

	data := make([]*cronpb.Plan, 0, len(plans))
	for _, plan := range plans {
		data = append(data, &cronpb.Plan{
			Id:         uint32(plan.ID),
			Name:       plan.Name,
			Enable:     plan.Enabled,
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
		Name:    req.Name,
		Plan:    req.Plan,
		Enabled: req.Enable,
	}

	if err := ct.DB.Create(plan).Error; err != nil {
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

	updates := map[string]interface{}{
		"name":    req.Name,
		"plan":    req.Plan,
		"enabled": req.Enable,
	}

	if err := ct.DB.Model(&existingPlan).Updates(updates).Error; err != nil {
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

	// 执行软删除
	if err := ct.DB.Delete(&existingPlan).Error; err != nil {
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

func (ct *CrontabPlugin) ListRecordPlanStreams(ctx context.Context, req *cronpb.ReqRecordPlanStreamList) (*cronpb.RecordPlanStreamResponseList, error) {
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
			pkg.ScopeRecordPlanID(uint(req.RecordPlanId)),
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

	data := make([]*cronpb.RecordPlanStream, 0, len(streams))
	for _, stream := range streams {
		data = append(data, &cronpb.RecordPlanStream{
			RecordPlanId: uint32(stream.RecordPlanID),
			StreamPath:   stream.StreamPath,
			Fragment:     stream.Fragment,
			FilePath:     stream.FilePath,
			CreatedAt:    timestamppb.New(stream.CreatedAt),
			UpdatedAt:    timestamppb.New(stream.UpdatedAt),
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

func (ct *CrontabPlugin) AddRecordPlanStream(ctx context.Context, req *cronpb.RecordPlanStream) (*cronpb.Response, error) {
	if req.RecordPlanId == 0 {
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

	// 检查录制计划是否存在
	var plan pkg.RecordPlan
	if err := ct.DB.First(&plan, req.RecordPlanId).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record plan not found",
		}, nil
	}

	// 检查是否已存在相同的记录
	var count int64
	searchModel := pkg.RecordPlanStream{
		RecordPlanID: uint(req.RecordPlanId),
		StreamPath:   req.StreamPath,
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

	stream := &pkg.RecordPlanStream{
		RecordPlanID: uint(req.RecordPlanId),
		StreamPath:   req.StreamPath,
		Fragment:     req.Fragment,
		FilePath:     req.FilePath,
	}

	if err := ct.DB.Create(stream).Error; err != nil {
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

func (ct *CrontabPlugin) UpdateRecordPlanStream(ctx context.Context, req *cronpb.RecordPlanStream) (*cronpb.Response, error) {
	if req.RecordPlanId == 0 {
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

	// 检查记录是否存在
	var existingStream pkg.RecordPlanStream
	searchModel := pkg.RecordPlanStream{
		RecordPlanID: uint(req.RecordPlanId),
		StreamPath:   req.StreamPath,
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

	if err := ct.DB.Save(&existingStream).Error; err != nil {
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

func (ct *CrontabPlugin) RemoveRecordPlanStream(ctx context.Context, req *cronpb.DeleteRecordPlanStreamRequest) (*cronpb.Response, error) {
	if req.RecordPlanId == 0 {
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

	// 检查记录是否存在
	var existingStream pkg.RecordPlanStream
	searchModel := pkg.RecordPlanStream{
		RecordPlanID: uint(req.RecordPlanId),
		StreamPath:   req.StreamPath,
	}
	if err := ct.DB.Where(&searchModel).First(&existingStream).Error; err != nil {
		return &cronpb.Response{
			Code:    404,
			Message: "record not found",
		}, nil
	}

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
