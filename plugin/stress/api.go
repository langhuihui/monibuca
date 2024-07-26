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

func (r *StressPlugin) pull(count int, format, url string, newFunc func() m7s.PullHandler) error {
	if i := r.pullers.Length; count > i {
		for j := i; j < count; j++ {
			puller, err := r.Pull(fmt.Sprintf("stress/%d", j), fmt.Sprintf(format, url))
			if err != nil {
				return err
			}
			go r.startPull(puller, newFunc())
		}
	} else if count < i {
		for j := i; j > count; j-- {
			r.pullers.Items[j-1].Stop(pkg.ErrStopFromAPI)
			r.pullers.Remove(r.pullers.Items[j-1])
		}
	}
	return nil
}

func (r *StressPlugin) push(count int, streamPath, format, remoteHost string, newFunc func() m7s.PushHandler) (err error) {
	if i := r.pushers.Length; count > i {
		for j := i; j < count; j++ {
			pusher, err := r.Push(streamPath, fmt.Sprintf(format, remoteHost, j))
			if err != nil {
				return err
			}
			go r.startPush(pusher, newFunc())
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
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtmp://%s/stress/%d", req.RemoteHost, rtmp.NewPushHandler)
}

func (r *StressPlugin) PushRTSP(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, "rtsp://%s/stress/%d", req.RemoteHost, rtsp.NewPushHandler)
}

func (r *StressPlugin) PullRTMP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtmp://%s", req.RemoteURL, rtmp.NewPullHandler)
}

func (r *StressPlugin) PullRTSP(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "rtsp://%s", req.RemoteURL, rtsp.NewPullHandler)
}

func (r *StressPlugin) PullHDL(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), "http://%s", req.RemoteURL, hdl.NewPullHandler)
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
