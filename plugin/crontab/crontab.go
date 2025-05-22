package plugin_crontab

import (
	"time"

	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/crontab/pkg"
)

type Crontab struct {
	task.TickTask
	ctp *CrontabPlugin
}

func (r *Crontab) GetTickInterval() time.Duration {
	return time.Minute
}

func (r *Crontab) Tick(any) {
	r.Info("开始检查录制计划")

	// 获取当前时间
	now := time.Now()
	// 计算当前是一周中的第几天(0-6, 0是周日)和当前小时(0-23)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // 将周日从0改为7，以便计算
	}
	hour := now.Hour()

	// 计算当前时间对应的位置索引
	// (weekday-1)*24 + hour 得到当前时间在144位字符串中的位置
	// weekday-1 是因为我们要从周一开始计算
	index := (weekday-1)*24 + hour

	// 查询所有启用的录制计划
	var plans []pkg.RecordPlan
	model := pkg.RecordPlan{
		Enabled: true,
	}
	if err := r.ctp.DB.Where(&model).Find(&plans).Error; err != nil {
		r.Error("查询录制计划失败:", err)
		return
	}

	// 遍历所有计划
	for _, plan := range plans {
		if len(plan.Plan) != 144 {
			r.Error("录制计划格式错误，plan长度应为144位:", plan.Name)
			continue
		}

		// 检查当前时间对应的位置是否为1
		if plan.Plan[index] == '1' {
			r.Info("检测到需要开启录像的计划:", plan.Name)
			// TODO: 在这里添加开启录像的逻辑
		}
	}
}
