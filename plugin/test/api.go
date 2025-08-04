package plugin_test

import (
	"context"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "m7s.live/v5/pb"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	testpb "m7s.live/v5/plugin/test/pb"
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
	pbTasks := make([]*testpb.TestTask, 0, len(tasks))
	for _, task := range tasks {
		pbTasks = append(pbTasks, &testpb.TestTask{
			Action: task.Action,
			Delay:  durationpb.New(task.Delay),
			Format: task.Format,
		})
	}
	return pbTasks
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

	// 转换为 protobuf 格式
	pbCases := make([]*testpb.TestCase, 0, len(allCases))
	for _, tc := range allCases {
		pbCases = append(pbCases, ToPBTestCase(tc))
	}

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
