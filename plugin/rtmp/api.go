package plugin_rtmp

import (
	"context"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	_, err = r.Push(req.StreamPath, req.RemoteURL)
	return &gpb.SuccessResponse{}, err
}
