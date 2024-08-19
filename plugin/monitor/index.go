package plugin_monitor

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/monitor/pb"
	monitor "m7s.live/m7s/v5/plugin/monitor/pkg"
	"os"
	"time"
)

var _ = m7s.InstallPlugin[MonitorPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)
var sessionID uint32

func init() {
	sessionID = uint32(os.Getpid()<<16) | uint32(uint16(time.Now().UnixNano()))
}

type MonitorPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	//columnstore *frostdb.ColumnStore
}

func (cfg *MonitorPlugin) taskDisposeListener(task *util.Task, mt util.IMarcoTask) func() {
	return func() {
		var th monitor.Task
		th.SessionID = sessionID
		th.TaskID = task.ID
		th.ParentID = mt.GetTask().ID
		th.StartTime = task.StartTime
		th.EndTime = time.Now()
		th.OwnerType = task.GetOwnerType()
		th.TaskType = task.GetTaskTypeID()
		th.Reason = task.StopReason().Error()
		cfg.DB.Create(&th)
	}
}

func (cfg *MonitorPlugin) monitorTask(mt util.IMarcoTask) {
	mt.OnTaskAdded(func(task util.ITask) {
		task.GetTask().OnDispose(cfg.taskDisposeListener(task.GetTask(), mt))
	})
	for t := range mt.RangeSubTask {
		t.OnDispose(cfg.taskDisposeListener(t.GetTask(), mt))
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
