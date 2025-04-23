package plugin_stress

import (
	"context"
	"fmt"
	"strings"

	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"

	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/v5"
	gpb "m7s.live/v5/pb"
	flv "m7s.live/v5/plugin/flv/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/v5/plugin/rtsp/pkg"
	"m7s.live/v5/plugin/stress/pb"
)

func (r *StressPlugin) pull(count int, url string, puller m7s.PullerFactory) (err error) {
	hasPlaceholder := strings.Contains(url, "%d")
	if i := r.pullers.Length; count > i {
		for j := i; j < count; j++ {
			conf := config.Pull{}
			if hasPlaceholder {
				conf.URL = fmt.Sprintf(url, j)
			} else {
				conf.URL = url
			}
			p := puller(conf)
			ctx := p.GetPullJob().Init(p, &r.Plugin, fmt.Sprintf("stress/%d", j), conf, nil)
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
			r.pullers.Items[j-1].Stop(task.ErrStopByUser)
			r.pullers.Remove(r.pullers.Items[j-1])
		}
	}
	return
}

func (r *StressPlugin) push(count int, streamPath, url string, pusher m7s.PusherFactory) (err error) {
	if i := r.pushers.Length; count > i {
		for j := i; j < count; j++ {
			p := pusher()
			ctx := p.GetPushJob().Init(p, &r.Plugin, streamPath, config.Push{URL: fmt.Sprintf(url, j)}, nil)
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
			r.pushers.Items[j-1].Stop(task.ErrStopByUser)
			r.pushers.Remove(r.pushers.Items[j-1])
		}
	}
	return
}

func (r *StressPlugin) StartPush(ctx context.Context, req *pb.PushRequest) (res *gpb.SuccessResponse, err error) {
	var pusher m7s.PusherFactory
	switch req.Protocol {
	case "rtmp":
		pusher = rtmp.NewPusher
	case "rtsp":
		pusher = rtsp.NewPusher
	default:
		return nil, fmt.Errorf("unsupport protocol %s", req.Protocol)
	}
	return &gpb.SuccessResponse{}, r.push(int(req.PushCount), req.StreamPath, req.RemoteURL, pusher)
}

func (r *StressPlugin) StartPull(ctx context.Context, req *pb.PullRequest) (res *gpb.SuccessResponse, err error) {
	var puller m7s.PullerFactory
	switch req.Protocol {
	case "rtmp":
		puller = rtmp.NewPuller
	case "rtsp":
		puller = rtsp.NewPuller
	case "flv":
		puller = flv.NewPuller
	case "mp4":
		puller = mp4.NewPuller
	default:
		return nil, fmt.Errorf("unsupport protocol %s", req.Protocol)
	}
	return &gpb.SuccessResponse{}, r.pull(int(req.PullCount), req.RemoteURL, puller)
}

func (r *StressPlugin) StopPush(ctx context.Context, req *emptypb.Empty) (res *gpb.SuccessResponse, err error) {
	for pusher := range r.pushers.Range {
		pusher.Stop(task.ErrStopByUser)
	}
	r.pushers.Clear()
	return &gpb.SuccessResponse{}, nil
}

func (r *StressPlugin) StopPull(ctx context.Context, req *emptypb.Empty) (res *gpb.SuccessResponse, err error) {
	for puller := range r.pullers.Range {
		puller.Stop(task.ErrStopByUser)
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
