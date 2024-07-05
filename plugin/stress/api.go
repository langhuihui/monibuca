package plugin_stress

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/m7s/v5"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/pkg"
	hdl "m7s.live/m7s/v5/plugin/hdl/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/m7s/v5/plugin/rtsp/pkg"
	"m7s.live/m7s/v5/plugin/stress/pb"
)

func (r *StressPlugin) PushRTMP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	l := r.pushers.Length
	for i := range req.PushCount {
		pusher, err := r.Push(req.StreamPath, fmt.Sprintf("rtmp://%s/stress/%d", req.RemoteHost, int(i)+l))
		if err != nil {
			return nil, err
		}
		go r.startPush(pusher, &rtmp.Client{})
	}
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) PushRTSP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	l := r.pushers.Length
	for i := range req.PushCount {
		pusher, err := r.Push(req.StreamPath, fmt.Sprintf("rtsp://%s/stress/%d", req.RemoteHost, int(i)+l))
		if err != nil {
			return nil, err
		}
		go r.startPush(pusher, &rtsp.Client{})
	}
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) PullRTMP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	i := r.pullers.Length
	for range req.PullCount {
		puller, err := r.Pull(fmt.Sprintf("stress/%d", i), fmt.Sprintf("rtmp://%s", req.RemoteURL))
		if err != nil {
			return nil, err
		}
		go r.startPull(puller, &rtmp.Client{})
		i++
	}
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) PullRTSP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	i := r.pullers.Length
	for range req.PullCount {
		puller, err := r.Pull(fmt.Sprintf("stress/%d", i), fmt.Sprintf("rtsp://%s", req.RemoteURL))
		if err != nil {
			return nil, err
		}
		go r.startPull(puller, &rtsp.Client{})
		i++
	}
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) PullHDL(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	i := r.pullers.Length
	for range req.PullCount {
		puller, err := r.Pull(fmt.Sprintf("stress/%d", i), fmt.Sprintf("http://%s", req.RemoteURL))
		if err != nil {
			return nil, err
		}
		go r.startPull(puller, hdl.NewHDLPuller())
		i++
	}
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) startPush(pusher *m7s.Pusher, handler m7s.PushHandler) {
	r.pushers.AddUnique(pusher)
	pusher.Start(handler)
	r.pushers.Remove(pusher)
}

func (r *StressPlugin) startPull(puller *m7s.Puller, handler m7s.PullHandler) {
	r.pullers.AddUnique(puller)
	puller.Start(handler)
	r.pullers.Remove(puller)
}

func (r *StressPlugin) StopPush(ctx context.Context, req *emptypb.Empty) (res *gpb.SuccessResponse, err error) {
	for pusher := range r.pushers.Range {
		pusher.Stop(pkg.ErrStopFromAPI)
	}
	r.pushers.Clear()
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) StopPull(ctx context.Context, req *emptypb.Empty) (res *gpb.SuccessResponse, err error) {
	for puller := range r.pullers.Range {
		puller.Stop(pkg.ErrStopFromAPI)
	}
	r.pullers.Clear()
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) GetCount(ctx context.Context, req *emptypb.Empty) (res *pb.CountResponse, err error) {
	return &pb.CountResponse{
		PullCount: uint32(r.pullers.Length),
		PushCount: uint32(r.pushers.Length),
	}, nil
}
