package plugin_monitor

import (
	"context"
	"errors"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5/plugin/monitor/pb"
	monitor "m7s.live/v5/plugin/monitor/pkg"
	"slices"
)

func (cfg *MonitorPlugin) SearchTask(ctx context.Context, req *pb.SearchTaskRequest) (res *pb.SearchTaskResponse, err error) {
	if cfg.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	res = &pb.SearchTaskResponse{}
	var tasks []*monitor.Task
	tx := cfg.DB.Find(&tasks)
	if err = tx.Error; err == nil {
		res.Data = slices.Collect(func(yield func(*pb.Task) bool) {
			for _, t := range tasks {
				if t.SessionID == req.SessionId {
					yield(&pb.Task{
						Id:          t.TaskID,
						StartTime:   timestamppb.New(t.StartTime),
						EndTime:     timestamppb.New(t.EndTime),
						Owner:       t.OwnerType,
						Type:        uint32(t.TaskType),
						Description: t.Description,
						Reason:      t.Reason,
						SessionId:   t.SessionID,
						ParentId:    t.ParentID,
					})
				}
			}
		})
	}
	return
}

func (cfg *MonitorPlugin) SessionList(context.Context, *emptypb.Empty) (res *pb.SessionListResponse, err error) {
	if cfg.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	res = &pb.SessionListResponse{}
	var sessions []*monitor.Session
	tx := cfg.DB.Find(&sessions)
	err = tx.Error
	if err == nil {
		res.Data = slices.Collect(func(yield func(*pb.Session) bool) {
			for _, s := range sessions {
				yield(&pb.Session{
					Id:        s.ID,
					Pid:       uint32(s.PID),
					Args:      s.Args,
					StartTime: timestamppb.New(s.StartTime),
					EndTime:   timestamppb.New(s.EndTime),
				})
			}
		})
	}
	return
}
