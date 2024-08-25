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

func (r *StressPlugin) pull(count int, format, url string, puller m7s.Puller) (err error) {
	if i := r.pullers.Length; count > i {
		for j := i; j < count; j++ {
			p := puller()
			ctx := p.GetPullJob().Init(p, &r.Plugin, fmt.Sprintf("stress/%d", j), fmt.Sprintf(format, url))
			if err = ctx.WaitStarted(); err != nil {
				return
			}
			r.pullers.AddUnique(ctx)
			ctx.OnDispose(func() {
				r.pullers.Remove(ctx)
			})
		}
	} else if count < i {
		for j := i; j > count; j-- {
			r.pullers.Items[j-1].Stop(pkg.ErrStopFromAPI)
			r.pullers.Remove(r.pullers.Items[j-1])
		}
	}
	return
}

func (r *StressPlugin) push(count int, streamPath, format, remoteHost string, pusher m7s.Pusher) (err error) {
	if i := r.pushers.Length; count > i {
		for j := i; j < count; j++ {
			p := pusher()
			ctx := p.GetPushJob().Init(p, &r.Plugin, streamPath, fmt.Sprintf(format, remoteHost, j))
			if err = ctx.WaitStarted(); err != nil {
				return
			}
			r.pushers.AddUnique(ctx)
			ctx.OnDispose(func() {
				r.pushers.Remove(ctx)
			})
		}
	} else if count < i {
		for j := i; j > count; j-- {
			r.pushers.Items[j-1].Stop(pkg.ErrStopFromAPI)
			r.pushers.Remove(r.pushers.Items[j-1])
		}
	}
	return
}

func (r *StressPlugin) PushRTMP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtmp://%s/stress/%d", req.RemoteHost, rtmp.NewPusher)
}

func (r *StressPlugin) PushRTSP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtsp://%s/stress/%d", req.RemoteHost, rtsp.NewPusher)
}

func (r *StressPlugin) PullRTMP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtmp://%s", req.RemoteURL, rtmp.NewPuller)
}

func (r *StressPlugin) PullRTSP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtsp://%s", req.RemoteURL, rtsp.NewPuller)
}

func (r *StressPlugin) PullHDL(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "http://%s", req.RemoteURL, hdl.NewPuller)
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
