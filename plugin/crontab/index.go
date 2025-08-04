package plugin_crontab

import (
	"fmt"

	"m7s.live/v5/pkg/util"

	"m7s.live/v5"
	"m7s.live/v5/plugin/crontab/pb"
	"m7s.live/v5/plugin/crontab/pkg"
)

type CrontabPlugin struct {
	m7s.Plugin
	pb.UnimplementedApiServer
	crontabs    util.Collection[string, *Crontab]
	recordPlans util.Collection[uint, *pkg.RecordPlan]
}

var _ = m7s.InstallPlugin[CrontabPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

func (ct *CrontabPlugin) Start() (err error) {
	if ct.DB == nil {
		ct.Error("DB is nil")
	} else {
		err = ct.DB.AutoMigrate(&pkg.RecordPlan{}, &pkg.RecordPlanStream{})
		if err != nil {
			return fmt.Errorf("auto migrate tables error: %v", err)
		}
		ct.Info("init database success")

		// 初始化默认录制计划（工作日和周末计划）
		ct.InitDefaultPlans()

		// 查询所有录制计划
		var plans []pkg.RecordPlan
		if err = ct.DB.Find(&plans).Error; err != nil {
			return fmt.Errorf("query record plans error: %v", err)
		}

		// 遍历所有计划
		for _, plan := range plans {
			// 将计划存入 recordPlans 集合
			ct.recordPlans.Add(&plan)

			// 如果计划已启用，查询对应的流信息并创建定时任务
			if plan.Enable {
				var streams []pkg.RecordPlanStream
				model := &pkg.RecordPlanStream{PlanID: plan.ID}
				if err = ct.DB.Model(model).Where(model).Find(&streams).Error; err != nil {
					ct.Error("query record plan streams error: %v", err)
					continue
				}

				// 为每个流创建定时任务
				for _, stream := range streams {
					crontab := &Crontab{
						ctp:              ct,
						RecordPlan:       &plan,
						RecordPlanStream: &stream,
					}
					crontab.OnStart(func() {
						ct.crontabs.Set(crontab)
					})
					ct.AddTask(crontab)
				}
			}
		}
	}
	return
}
