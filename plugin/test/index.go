package plugin_test

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/test/pb"
)

const (
	TestCaseStatusInit     TestCaseStatus = "init"
	TestCaseStatusStarting TestCaseStatus = "starting"
	TestCaseStatusRunning  TestCaseStatus = "running"
	TestCaseStatusSuccess  TestCaseStatus = "success"
	TestCaseStatusFailed   TestCaseStatus = "failed"
)

func (f *TestTaskFactory) Register(action string, taskCreator func(*TestCase, TestTaskConfig) task.ITask) {
	f.tasks[action] = taskCreator
}

func (f *TestTaskFactory) Create(taskConfig TestTaskConfig, scenario *TestCase) (task.ITask, error) {
	if taskCreator, exists := f.tasks[taskConfig.Action]; exists {
		return taskCreator(scenario, taskConfig), nil
	}
	return nil, fmt.Errorf("no task registered for action: %s", taskConfig)
}

var testTaskFactory = TestTaskFactory{
	tasks: make(map[string]func(*TestCase, TestTaskConfig) task.ITask),
}

type (
	TestTaskFactory struct {
		tasks map[string]func(*TestCase, TestTaskConfig) task.ITask
	}
	TestTaskConfig struct {
		Action     string        `json:"action"`
		Delay      time.Duration `json:"delay"`
		Format     string        `json:"format"`
		ServerAddr string        `json:"serverAddr" default:"localhost"`
		Input      string        `json:"input"`
		StreamPath string        `json:"streamPath"`
	}
	TestCaseStatus string
	TestConfig     struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		VideoCodec  string           `json:"videoCodec" default:"h264"`
		AudioCodec  string           `json:"audioCodec" default:"aac"`
		VideoOnly   bool             `json:"videoOnly"`
		AudioOnly   bool             `json:"audioOnly"`
		Tags        []string         `json:"tags"`
		Timeout     time.Duration    `json:"timeout" default:"30s"`
		Tasks       []TestTaskConfig `json:"tasks"`
	}
	TestCase struct {
		*task.Job `json:"-"`
		*TestConfig
		Plugin    *TestPlugin    `json:"-"`
		Status    TestCaseStatus `json:"status"`
		StartTime int64          `json:"startTime"`
		EndTime   int64          `json:"endTime"`
		Duration  int32          `json:"duration"`
		ErrorMsg  string         `json:"errorMsg"`
		Logs      string         `json:"logs"`
	}
	TestPlugin struct {
		pb.UnimplementedApiServer
		m7s.Plugin
		Cases     map[string]TestConfig
		testCases map[string]*TestCase
		flushSSE  chan struct{}
	}
	TestBaseTask struct {
		task.Task
		testCase *TestCase
		TestTaskConfig
	}
)

func (ts *TestCase) Start() (err error) {
	ts.Status = TestCaseStatusStarting
	ts.StartTime = time.Now().Unix()
	return nil
}

func (ts *TestCase) Go() (err error) {
	ts.Status = TestCaseStatusRunning
	ts.Plugin.FlushSSE()
	subTaskSelect := []reflect.SelectCase{
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(time.After(ts.Timeout)),
		},
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ts.Done()),
		},
	}
	var subTask []task.ITask
	for _, taskConfig := range ts.Tasks {
		if taskConfig.StreamPath == "" {
			taskConfig.StreamPath = fmt.Sprintf("test/%d", ts.ID)
		}
		if taskConfig.Input != "" && !strings.Contains(taskConfig.Input, ".") {
			taskConfig.Input = fmt.Sprintf("%s/%d", taskConfig.Input, ts.ID)
		}
		t, err := testTaskFactory.Create(taskConfig, ts)
		if err != nil {
			ts.Status = TestCaseStatusFailed
			ts.ErrorMsg = fmt.Sprintf("Failed to create test task: %v", err)
			ts.Plugin.FlushSSE()
			return err
		}
		if taskConfig.Delay > 0 {
			subTask = append(subTask, t)
			subTaskSelect = append(subTaskSelect, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(time.After(taskConfig.Delay)),
			})
		} else {
			ts.AddDependTask(t)
		}
	}
	for {
		chosen, _, recvOK := reflect.Select(subTaskSelect)
		switch chosen {
		case 0:
			ts.Stop(task.ErrTimeout)
		case 1:
			if errors.Is(ts.StopReason(), task.ErrTaskComplete) {
				ts.Status = TestCaseStatusSuccess
			} else {
				ts.Status = TestCaseStatusFailed
				ts.ErrorMsg = ts.StopReason().Error()
			}
			ts.Plugin.FlushSSE()
			return nil
		default:
			if recvOK {
				ts.AddDependTask(subTask[chosen-2])
			}
		}
	}
}

// Dispose 任务停止
func (ts *TestCase) Dispose() {
	if ts.ErrorMsg == "" {
		ts.Status = TestCaseStatusSuccess
	} else {
		ts.Status = TestCaseStatusFailed
	}
	ts.EndTime = time.Now().Unix()
	ts.Duration = int32(time.Now().Unix() - ts.StartTime)
	ts.Plugin.FlushSSE()
}

func (ts *TestCase) Write(buf []byte) (int, error) {
	ts.Logs += time.Now().Format("2006-01-02 15:04:05") + " " + string(buf) + "\n"
	return len(buf), nil
}

// GetTestCaseFromCache 从缓存获取测试用例
func (p *TestPlugin) GetTestCaseFromCache(name string) (tc *TestCase, exists bool) {
	p.Call(func() {
		tc, exists = p.testCases[name]
	})
	return
}

func (p *TestPlugin) FlushSSE() {
	select {
	case p.flushSSE <- struct{}{}:
	default:
	}
}

type TestCaseFilter struct {
	Tags     []string
	Status   TestCaseStatus
	Category string
	TestType string
}

var StatusOrder = [...]TestCaseStatus{
	TestCaseStatusRunning,
	TestCaseStatusStarting,
	TestCaseStatusFailed,
	TestCaseStatusSuccess,
	TestCaseStatusInit,
}

func (p *TestPlugin) GetTestCasesFromCache(filter TestCaseFilter) (cases []*TestCase) {
	p.Call(func() {
		for _, tc := range p.testCases {
			// 标签过滤
			if len(filter.Tags) > 0 {
				if !slices.ContainsFunc(filter.Tags, func(tag string) bool {
					return slices.Contains(tc.Tags, tag)
				}) {
					continue
				}
			}

			if filter.Status != "" && tc.Status != filter.Status {
				continue
			}
			cases = append(cases, tc)
		}
	})
	slices.SortFunc(cases, func(a, b *TestCase) int {
		if a.Status == b.Status {
			return strings.Compare(a.Name, b.Name)
		}
		for _, status := range StatusOrder {
			if a.Status == status {
				return -1
			}
			if b.Status == status {
				return 1
			}
		}
		return 0
	})
	return
}

//go:embed default.yaml
var defaultYaml m7s.DefaultYaml

var _ = m7s.InstallPlugin[TestPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
	DefaultYaml:         defaultYaml,
})

func (p *TestPlugin) Start() error {
	p.testCases = make(map[string]*TestCase)
	for name, tc := range p.Cases {
		tc.Name = name
		p.testCases[name] = &TestCase{
			TestConfig: &tc,
			Plugin:     p,
			Status:     TestCaseStatusInit,
		}
	}
	p.flushSSE = make(chan struct{}, 1)
	return nil
}

func (p *TestPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/sse/cases": p.GetTestCaseSSE,
	}
}
