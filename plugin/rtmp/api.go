package plugin_rtmp

import (
	"context"
	gpb "m7s.live/v5/pb"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/plugin/rtmp/pb"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	pusher := rtmp.NewPusher()
	err = pusher.GetPushJob().Init(pusher, &r.Plugin, req.StreamPath, config.Push{URL: req.RemoteURL}).WaitStarted()
	return &gpb.SuccessResponse{}, err
}
