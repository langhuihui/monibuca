package plugin_rtmp

import (
	"context"
	gpb "m7s.live/pro/pb"
	"m7s.live/pro/pkg/config"
	"m7s.live/pro/plugin/rtmp/pb"
	rtmp "m7s.live/pro/plugin/rtmp/pkg"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	pusher := rtmp.NewPusher()
	err = pusher.GetPushJob().Init(pusher, &r.Plugin, req.StreamPath, config.Push{URL: req.RemoteURL}).WaitStarted()
	return &gpb.SuccessResponse{}, err
}
