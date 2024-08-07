package plugin_rtmp

import (
	"context"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/plugin/rtmp/pb"
)

func (r *RTMPPlugin) PushOut(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	if pushContext, err := r.Push(req.StreamPath, req.RemoteURL); err != nil {
		return nil, err
	} else {
		go pushContext.Run(r.DoPush)
	}
	return &gpb.SuccessResponse{}, err
}
