package plugin_crontab

import (
	"fmt"

	"m7s.live/v5"
	"m7s.live/v5/plugin/crontab/pb"
	"m7s.live/v5/plugin/crontab/pkg"
)

type CrontabPlugin struct {
	m7s.Plugin
	pb.UnimplementedApiServer
}

var _ = m7s.InstallPlugin[CrontabPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

func (ct *CrontabPlugin) OnInit() (err error) {
	if ct.DB == nil {
		ct.Error("DB is nil")
	} else {
		err = ct.DB.AutoMigrate(&pkg.RecordPlan{}, &pkg.RecordPlanStream{})
		if err != nil {
			return fmt.Errorf("auto migrate tables error: %v", err)
		}
		ct.Info("init database success")
	}
	crontab := &Crontab{ctp: ct}
	ct.AddTask(crontab)
	crontab.Tick(nil)
	return
}
