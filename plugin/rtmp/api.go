package plugin_rtmp

import (
	"context"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	go r.Push(req.StreamPath, req.RemoteURL, &rtmp.Client{})
	return &gpb.SuccessResponse{}, nil
}
