package plugin_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mcuadros/go-defaults"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5"
	pb "m7s.live/v5/pb"
	"m7s.live/v5/pkg/collections"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
	hls "m7s.live/v5/plugin/hls/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/v5/plugin/rtsp/pkg"
	srt "m7s.live/v5/plugin/srt/pkg"
	testpb "m7s.live/v5/plugin/test/pb"
	webrtc "m7s.live/v5/plugin/webrtc/pkg"
)

// ========== Protobuf 转换函数 ========== //

// ToPBTestCase 转换为 protobuf TestCase
func ToPBTestCase(tc *TestCase) *testpb.TestCase {
	if tc == nil {
		return nil
	}
	return &testpb.TestCase{
		Name:        tc.Name,
		Description: tc.Description,
		Timeout:     durationpb.New(tc.Timeout),
		Tasks:       ToPBTestTasks(tc.Tasks),
		Status:      string(tc.Status),
		StartTime:   timestamppb.New(time.Unix(tc.StartTime, 0)),
		EndTime:     timestamppb.New(time.Unix(tc.EndTime, 0)),
		Duration:    tc.Duration,
		VideoCodec:  tc.VideoCodec,
		AudioCodec:  tc.AudioCodec,
		VideoOnly:   tc.VideoOnly,
		AudioOnly:   tc.AudioOnly,
		ErrorMsg:    tc.ErrorMsg,
		Logs:        tc.Logs,
		Tags:        tc.Tags,
	}
}

func ToPBTestTasks(tasks []TestTaskConfig) []*testpb.TestTask {
	return collections.Map(tasks, func(task TestTaskConfig) *testpb.TestTask {
		return &testpb.TestTask{
			Action: task.Action,
			Delay:  durationpb.New(task.Delay),
			Format: task.Format,
		}
	})
}

// ========== Protobuf Gateway API 实现 ========== //

// ListTestCases 获取测试用例列表
func (p *TestPlugin) ListTestCases(ctx context.Context, req *testpb.ListTestCasesRequest) (*testpb.ListTestCasesResponse, error) {
	// 构建过滤器
	filter := TestCaseFilter{
		Tags:   req.Tags,
		Status: TestCaseStatus(req.Status),
	}
	// 从缓存获取测试用例
	allCases := p.GetTestCasesFromCache(filter)

	pbCases := collections.Map(allCases, ToPBTestCase)

	return &testpb.ListTestCasesResponse{
		Code: 0, Message: "success", Data: pbCases,
	}, nil
}

func (p *TestPlugin) ExecuteTestCase(ctx context.Context, req *testpb.ExecuteTestCaseRequest) (*pb.SuccessResponse, error) {
	for _, name := range req.Names {
		tc, exists := p.GetTestCaseFromCache(name)
		if !exists || tc.Status == TestCaseStatusRunning || tc.Status == TestCaseStatusStarting {
			continue
		}
		tc.Job = &task.Job{}
		tc.ErrorMsg = ""
		tc.Logs = ""
		p.AddTask(tc)
	}
	return &pb.SuccessResponse{Code: 0, Message: "success"}, nil
}

func (p *TestPlugin) GetTestCaseSSE(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var filter TestCaseFilter
	tags := query.Get("tags")
	if tags != "" {
		filter.Tags = strings.Split(tags, ",")
	}
	filter.Status = TestCaseStatus(query.Get("status"))
	util.NewSSE(w, r.Context(), func(sse *util.SSE) {
		flush := func() error {
			return sse.WriteJSON(p.GetTestCasesFromCache(filter))
		}
		if err := flush(); err != nil {
			return
		}
		// 创建定时器，定期发送状态更新
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sse.Context.Done():
				return
			case <-p.flushSSE:
				if err := flush(); err != nil {
					return
				}
			case <-ticker.C:
				if err := flush(); err != nil {
					return
				}
			}
		}
	})
}

// ========== Stress 测试相关 API 实现 ========== //

func (p *TestPlugin) pull(count int, url string, testMode int32, puller m7s.PullerFactory) (err error) {
	hasPlaceholder := strings.Contains(url, "%d")
	if i := p.pullers.Length; count > i {
		for j := i; j < count; j++ {
			conf := config.Pull{}
			defaults.SetDefaults(&conf)
			conf.TestMode = int(testMode)
			if hasPlaceholder {
				conf.URL = fmt.Sprintf(url, j)
			} else {
				conf.URL = url
			}
			puller := puller(conf)
			ctx := puller.GetPullJob().Init(puller, &p.Plugin, fmt.Sprintf("stress/%d", j), conf, nil)
			if err = ctx.WaitStarted(); err != nil {
				return
			}
			if old, ok := p.pullers.Get(ctx.GetKey()); ok {
				ctx.Stop(task.ExistTaskError{
					Task: old,
				})
			} else {
				p.pullers.Add(ctx)
				ctx.OnDispose(func() {
					p.pullers.Remove(ctx)
				})
			}
		}
	} else if count < i {
		clone := slices.Clone(p.pullers.Items)
		for j := i; j > count; j-- {
			clone[j-1].Stop(task.ErrStopByUser)
		}
	}
	return
}

func (p *TestPlugin) push(count int, streamPath, url string, pusher m7s.PusherFactory) (err error) {
	if i := p.pushers.Length; count > i {
		for j := i; j < count; j++ {
			pusher := pusher()
			conf := config.Push{URL: fmt.Sprintf(url, j)}
			defaults.SetDefaults(&conf)
			ctx := pusher.GetPushJob().Init(pusher, &p.Plugin, streamPath, conf, nil)
			if err = ctx.WaitStarted(); err != nil {
				return
			}
			if old, ok := p.pushers.Get(ctx.GetKey()); ok {
				ctx.Stop(task.ExistTaskError{
					Task: old,
				})
			} else {
				p.pushers.Add(ctx)
				ctx.OnDispose(func() {
					p.pushers.Remove(ctx)
				})
			}
		}
	} else if count < i {
		clone := slices.Clone(p.pushers.Items)
		for j := i; j > count; j-- {
			clone[j-1].Stop(task.ErrStopByUser)
		}
	}
	return
}

func (p *TestPlugin) StartPush(ctx context.Context, req *testpb.PushRequest) (res *pb.SuccessResponse, err error) {
	var pusher m7s.PusherFactory
	if req.Protocol == "" {
		if strings.HasPrefix(req.RemoteURL, "http") {
			req.Protocol = "webrtc"
		} else if strings.HasPrefix(req.RemoteURL, "srt") {
			req.Protocol = "srt"
		} else if strings.HasPrefix(req.RemoteURL, "rtsp") {
			req.Protocol = "rtsp"
		} else if strings.HasPrefix(req.RemoteURL, "rtmp") {
			req.Protocol = "rtmp"
		} else {
			return nil, fmt.Errorf("unsupport protocol %s", req.RemoteURL)
		}
	}
	switch req.Protocol {
	case "rtmp":
		pusher = rtmp.NewPusher
	case "rtsp":
		pusher = rtsp.NewPusher
	case "srt":
		pusher = srt.NewPusher
	case "webrtc":
		pusher = webrtc.NewPusher
	default:
		return nil, fmt.Errorf("unsupport protocol %s", req.Protocol)
	}
	return &pb.SuccessResponse{}, p.push(int(req.PushCount), req.StreamPath, req.RemoteURL, pusher)
}

func (p *TestPlugin) StartPull(ctx context.Context, req *testpb.PullRequest) (res *pb.SuccessResponse, err error) {
	var puller m7s.PullerFactory
	if req.Protocol == "" {
		u, err := url.Parse(req.RemoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse remote url failed: %w", err)
		}
		if strings.HasSuffix(u.Path, ".m3u8") {
			req.Protocol = "hls"
		} else if strings.HasSuffix(u.Path, ".flv") {
			req.Protocol = "flv"
		} else if strings.HasSuffix(u.Path, ".mp4") {
			req.Protocol = "mp4"
		} else if strings.HasPrefix(req.RemoteURL, "srt") {
			req.Protocol = "srt"
		} else if strings.HasPrefix(req.RemoteURL, "rtsp") {
			req.Protocol = "rtsp"
		} else if strings.HasPrefix(req.RemoteURL, "rtmp") {
			req.Protocol = "rtmp"
		} else {
			req.Protocol = "webrtc"
		}
	}
	switch req.Protocol {
	case "rtmp":
		puller = rtmp.NewPuller
	case "rtsp":
		puller = rtsp.NewPuller
	case "srt":
		puller = srt.NewPuller
	case "flv":
		puller = flv.NewPuller
	case "mp4":
		puller = mp4.NewPuller
	case "webrtc":
		puller = webrtc.NewPuller
	case "hls":
		puller = hls.NewPuller
	default:
		return nil, fmt.Errorf("unsupport protocol %s", req.Protocol)
	}
	return &pb.SuccessResponse{}, p.pull(int(req.PullCount), req.RemoteURL, req.TestMode, puller)
}

func (p *TestPlugin) StopPush(ctx context.Context, req *emptypb.Empty) (res *pb.SuccessResponse, err error) {
	for _, pusher := range slices.Clone(p.pushers.Items) {
		pusher.Stop(task.ErrStopByUser)
	}
	return &pb.SuccessResponse{}, nil
}

func (p *TestPlugin) StopPull(ctx context.Context, req *emptypb.Empty) (res *pb.SuccessResponse, err error) {
	for _, puller := range slices.Clone(p.pullers.Items) {
		puller.Stop(task.ErrStopByUser)
	}
	return &pb.SuccessResponse{}, nil
}

func (p *TestPlugin) GetCount(ctx context.Context, req *emptypb.Empty) (res *testpb.CountResponse, err error) {
	return &testpb.CountResponse{
		Data: &testpb.CountResponseData{
			PullCount: uint32(p.pullers.Length),
			PushCount: uint32(p.pushers.Length),
		},
	}, nil
}
