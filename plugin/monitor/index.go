package plugin_monitor

import (
	"encoding/json"
	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/monitor/pb"
	monitor "m7s.live/v5/plugin/monitor/pkg"
	"os"
	"strings"
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

func (cfg *MonitorPlugin) saveTask(task task.ITask) {
	var th monitor.Task
	th.SessionID = cfg.session.ID
	th.TaskID = task.GetTaskID()
	th.ParentID = task.GetParent().GetTaskID()
	th.StartTime = task.GetTask().StartTime
	th.EndTime = time.Now()
	th.OwnerType = task.GetOwnerType()
	th.TaskType = byte(task.GetTaskType())
	th.Reason = task.StopReason().Error()
	th.Level = task.GetLevel()
	b, _ := json.Marshal(task.GetDescriptions())
	th.Description = string(b)
	cfg.DB.Create(&th)
}

func (cfg *MonitorPlugin) OnInit() (err error) {
	//cfg.columnstore, err = frostdb.New()
	//database, _ := cfg.columnstore.DB(cfg, "monitor")
	if cfg.DB != nil {
		session := &monitor.Session{StartTime: time.Now(), PID: os.Getpid(), Args: strings.Join(os.Args, " ")}
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
		cfg.Plugin.Server.OnBeforeDispose(func() {
			cfg.saveTask(cfg.Plugin.Server)
		})
		cfg.Plugin.Server.OnChildDispose(cfg.saveTask)
	}
	return
}
