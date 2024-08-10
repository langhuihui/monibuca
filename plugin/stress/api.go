package plugin_stress

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/m7s/v5"
	gpb "m7s.live/m7s/v5/pb"
	"m7s.live/m7s/v5/pkg"
	hdl "m7s.live/m7s/v5/plugin/flv/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/m7s/v5/plugin/rtsp/pkg"
	"m7s.live/m7s/v5/plugin/stress/pb"
)

func (r *StressPlugin) pull(count int, format, url string, puller m7s.Puller) error {
	if i := r.pullers.Length; count > i {
		for j := i; j < count; j++ {
			ctx, err := r.Pull(fmt.Sprintf("stress/%d", j), fmt.Sprintf(format, url))
			if err != nil {
				return err
			}
			ctx.AddCall(func(*pkg.Task) error {
				r.pullers.AddUnique(ctx)
				ctx.Do(puller)
				return nil
			}, func(*pkg.Task) {
				r.pullers.Remove(ctx)
			})
		}
	} else if count < i {
		for j := i; j > count; j-- {
			r.pullers.Items[j-1].Stop(pkg.ErrStopFromAPI)
			r.pullers.Remove(r.pullers.Items[j-1])
		}
	}
	return nil
}

func (r *StressPlugin) push(count int, streamPath, format, remoteHost string, pusher m7s.Pusher) (err error) {
	if i := r.pushers.Length; count > i {
		for j := i; j < count; j++ {
			ctx, err := r.Push(streamPath, fmt.Sprintf(format, remoteHost, j))
			if err != nil {
				return err
			}
			ctx.AddCall(func(*pkg.Task) error {
				r.pushers.AddUnique(ctx)
				ctx.Do(pusher)
				return nil
			}, func(*pkg.Task) {
				r.pushers.Remove(ctx)
			})
		}
	} else if count < i {
		for j := i; j > count; j-- {
			r.pushers.Items[j-1].Stop(pkg.ErrStopFromAPI)
			r.pushers.Remove(r.pushers.Items[j-1])
		}
	}
	return nil
}

func (r *StressPlugin) PushRTMP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtmp://%s/stress/%d", req.RemoteHost, rtmp.Push)
}

func (r *StressPlugin) PushRTSP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtsp://%s/stress/%d", req.RemoteHost, rtsp.Push)
}

func (r *StressPlugin) PullRTMP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtmp://%s", req.RemoteURL, rtmp.Pull)
}

func (r *StressPlugin) PullRTSP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtsp://%s", req.RemoteURL, rtsp.Pull)
}

func (r *StressPlugin) PullHDL(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "http://%s", req.RemoteURL, hdl.PullFLV)
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
