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
