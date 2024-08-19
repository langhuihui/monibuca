package plugin_rtmp

import (
	"context"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	pusher := rtmp.NewPusher()
	err = r.Server.AddPushTask(pusher.GetPushContext().Init(pusher, &r.Plugin, req.StreamPath, req.RemoteURL)).WaitStarted()
	return &gpb.SuccessResponse{}, err
}
