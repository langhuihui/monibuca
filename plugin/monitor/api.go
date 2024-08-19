package plugin_monitor

import (
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/m7s/v5/plugin/monitor/pb"
	monitor "m7s.live/m7s/v5/plugin/monitor/pkg"
	"slices"
)

func (cfg *MonitorPlugin) SearchTask(ctx context.Context, req *pb.SearchTaskRequest) (res *pb.SearchTaskResponse, err error) {
	res = &pb.SearchTaskResponse{}
	var tasks []*monitor.Task
	tx := cfg.DB.Find(&tasks)
	if err = tx.Error; err == nil {
		res.Data = slices.Collect(func(yield func(*pb.Task) bool) {
			for _, t := range tasks {
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
		})
	}
	return
}
