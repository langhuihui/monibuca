package plugin_monitor

import (
	"encoding/json"
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
	session *monitor.Session
	//columnstore *frostdb.ColumnStore
}

func (cfg *MonitorPlugin) OnStop() {
	if cfg.DB != nil {
		//cfg.saveUnDisposeTask(cfg.Plugin.Server)
		cfg.DB.Model(cfg.session).Update("end_time", time.Now())
	}
}

func (cfg *MonitorPlugin) taskDisposeListener(task util.ITask, mt util.IMarcoTask) func() {
	return func() {
		var th monitor.Task
		th.SessionID = cfg.session.ID
		th.TaskID = task.GetTask().ID
		th.ParentID = mt.GetTask().ID
		th.StartTime = task.GetTask().StartTime
		th.EndTime = time.Now()
		th.OwnerType = task.GetOwnerType()
		th.TaskType = task.GetTaskTypeID()
		th.Reason = task.StopReason().Error()
		b, _ := json.Marshal(task.GetTask().Description)
		th.Description = string(b)
		cfg.DB.Create(&th)
	}
}

func (cfg *MonitorPlugin) monitorTask(mt util.IMarcoTask) {
	mt.OnTaskAdded(func(task util.ITask) {
		task.GetTask().OnDispose(cfg.taskDisposeListener(task, mt))
	})
	for t := range mt.RangeSubTask {
		t.OnDispose(cfg.taskDisposeListener(t, mt))
		if mt, ok := t.(util.IMarcoTask); ok {
			cfg.monitorTask(mt)
		}
	}
}

//func (cfg *MonitorPlugin) saveUnDisposeTask(mt util.IMarcoTask) {
//	for t := range mt.RangeSubTask {
//		cfg.taskDisposeListener(t, mt)()
//		if mt, ok := t.(util.IMarcoTask); ok {
//			cfg.saveUnDisposeTask(mt)
//		}
//	}
//}

func (cfg *MonitorPlugin) OnInit() (err error) {
	//cfg.columnstore, err = frostdb.New()
	//database, _ := cfg.columnstore.DB(cfg, "monitor")
	if cfg.DB != nil {
		session := &monitor.Session{StartTime: time.Now()}
		err = cfg.DB.AutoMigrate(session)
		if err != nil {
			return err
		}
		err = cfg.DB.Create(session).Error
		if err != nil {
			return err
		}
		cfg.session = session
		cfg.Info("monitor session start", "session", session.ID)
		err = cfg.DB.AutoMigrate(&monitor.Task{})
		if err != nil {
			return err
		}
		cfg.monitorTask(cfg.Plugin.Server)
	}
	return
}
