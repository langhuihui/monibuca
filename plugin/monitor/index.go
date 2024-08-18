package plugin_monitor

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/monitor/pb"
	monitor "m7s.live/m7s/v5/plugin/monitor/pkg"
	"time"
)

var _ = m7s.InstallPlugin[MonitorPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)

type MonitorPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	//columnstore *frostdb.ColumnStore
}

func (cfg *MonitorPlugin) taskDisposeListener(task *util.Task) func() {
	return func() {
		var th monitor.Task
		th.ID = task.ID
		th.StartTime = task.StartTime
		th.CreatedAt = time.Now()
		th.OwnerType = task.GetOwnerType()
		th.TaskType = task.GetTaskTypeID()
		th.Reason = task.StopReason().Error()
		cfg.DB.Create(&th)
	}
}

func (cfg *MonitorPlugin) monitorTask(mt util.IMarcoTask) {
	mt.OnTaskAdded(func(task util.ITask) {
		task.GetTask().OnDispose(cfg.taskDisposeListener(task.GetTask()))
	})
	for t := range mt.RangeSubTask {
		t.OnDispose(cfg.taskDisposeListener(t.GetTask()))
		if mt, ok := t.(util.IMarcoTask); ok {
			cfg.monitorTask(mt)
		}
	}
}

func (cfg *MonitorPlugin) OnInit() (err error) {
	//cfg.columnstore, err = frostdb.New()
	//database, _ := cfg.columnstore.DB(cfg, "monitor")
	if cfg.DB != nil {
		err = cfg.DB.AutoMigrate(&monitor.Task{})
		if err != nil {
			return err
		}
		cfg.monitorTask(cfg.Plugin.Server)
	}
	return
}
