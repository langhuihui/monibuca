package plugin_crontab

import (
	"strings"

	"m7s.live/v5/plugin/crontab/pkg"
)

// InitDefaultPlans 初始化默认的录制计划
// 包括工作日录制计划和周末录制计划
func (ct *CrontabPlugin) InitDefaultPlans() {
	// 创建工作日录制计划（周一到周五全天录制）的计划字符串
	workdayPlanStr := buildPlanString(false, true, true, true, true, true, false) // 周一到周五

	// 检查是否已存在相同内容的工作日录制计划
	var count int64
	if err := ct.DB.Model(&pkg.RecordPlan{}).Where("plan = ?", workdayPlanStr).Count(&count).Error; err != nil {
		ct.Error("检查工作日录制计划失败: %v", err)
	} else if count == 0 {
		// 不存在相同内容的计划，创建新计划
		workdayPlan := &pkg.RecordPlan{
			Name:   "工作日录制计划",
			Plan:   workdayPlanStr,
			Enable: true,
		}

		if err := ct.DB.Create(workdayPlan).Error; err != nil {
			ct.Error("创建工作日录制计划失败: %v", err)
		} else {
			ct.Info("成功创建工作日录制计划")
			// 添加到内存中
			ct.recordPlans.Add(workdayPlan)
		}
	} else {
		ct.Info("已存在相同内容的工作日录制计划，跳过创建")
	}

	// 创建周末录制计划（周六和周日全天录制）的计划字符串
	weekendPlanStr := buildPlanString(true, false, false, false, false, false, true) // 周日和周六

	// 检查是否已存在相同内容的周末录制计划
	if err := ct.DB.Model(&pkg.RecordPlan{}).Where("plan = ?", weekendPlanStr).Count(&count).Error; err != nil {
		ct.Error("检查周末录制计划失败: %v", err)
	} else if count == 0 {
		// 不存在相同内容的计划，创建新计划
		weekendPlan := &pkg.RecordPlan{
			Name:   "周末录制计划",
			Plan:   weekendPlanStr,
			Enable: true,
		}

		if err := ct.DB.Create(weekendPlan).Error; err != nil {
			ct.Error("创建周末录制计划失败: %v", err)
		} else {
			ct.Info("成功创建周末录制计划")
			// 添加到内存中
			ct.recordPlans.Add(weekendPlan)
		}
	} else {
		ct.Info("已存在相同内容的周末录制计划，跳过创建")
	}
}

// buildPlanString 构建计划字符串
// 参数分别表示：周日、周一、周二、周三、周四、周五、周六是否录制
// 返回168位的计划字符串，每天24小时，一周7天
func buildPlanString(sun, mon, tue, wed, thu, fri, sat bool) string {
	var planBuilder strings.Builder

	// 按照周日、周一、...、周六的顺序
	days := []bool{sun, mon, tue, wed, thu, fri, sat}
	
	for _, record := range days {
		if record {
			// 该天录制，24小时都为1
			planBuilder.WriteString(strings.Repeat("1", 24))
		} else {
			// 该天不录制，24小时都为0
			planBuilder.WriteString(strings.Repeat("0", 24))
		}
	}

	return planBuilder.String()
}
