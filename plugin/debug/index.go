package plugin_debug

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	runtimePPROF "runtime/pprof"
	"sort"
	"strings"
	"time"

	myproc "github.com/cloudwego/goref/pkg/proc"
	"github.com/go-delve/delve/pkg/config"
	"github.com/go-delve/delve/service/debugger"
	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/v5"
	"m7s.live/v5/plugin/debug/pb"
	debug "m7s.live/v5/plugin/debug/pkg"
	"m7s.live/v5/plugin/debug/pkg/profile"
)

var _ = m7s.InstallPlugin[DebugPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)
var conf, _ = config.LoadConfig()

type DebugPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	ProfileDuration time.Duration `default:"10s" desc:"profile持续时间"`
	Profile         string        `desc:"采集profile存储文件"`
	ChartPeriod     time.Duration `default:"1s" desc:"图表更新周期"`
	Grfout          string        `default:"grf.out" desc:"grf输出文件"`
}

type WriteToFile struct {
	header http.Header
	io.Writer
}

func (w *WriteToFile) Header() http.Header {
	// return w.w.Header()
	return w.header
}

//	func (w *WriteToFile) Write(p []byte) (int, error) {
//		// w.w.Write(p)
//		return w.Writer.Write(p)
//	}
func (w *WriteToFile) WriteHeader(statusCode int) {
	// w.w.WriteHeader(statusCode)
}

func (p *DebugPlugin) OnInit() error {
	// 启用阻塞分析
	runtime.SetBlockProfileRate(1) // 设置采样率为1纳秒

	if p.Profile != "" {
		go func() {
			file, err := os.Create(p.Profile)
			if err != nil {
				return
			}
			defer file.Close()
			p.Info("cpu profile start")
			err = runtimePPROF.StartCPUProfile(file)
			time.Sleep(p.ProfileDuration)
			runtimePPROF.StopCPUProfile()
			p.Info("cpu profile done")
		}()
	}
	return nil
}

func (p *DebugPlugin) Pprof_Trace(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Trace(w, r)
}

func (p *DebugPlugin) Pprof_profile(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Profile(w, r)
}

func (p *DebugPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/pprof" {
		http.Redirect(w, r, "/debug/pprof/", http.StatusFound)
		return
	}
	r.URL.Path = "/debug" + r.URL.Path
	pprof.Index(w, r)
}

func (p *DebugPlugin) Charts_(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/static" + strings.TrimPrefix(r.URL.Path, "/charts")
	staticFSHandler.ServeHTTP(w, r)
}

func (p *DebugPlugin) Charts_data(w http.ResponseWriter, r *http.Request) {
	dataHandler(w, r)
}

func (p *DebugPlugin) Charts_datafeed(w http.ResponseWriter, r *http.Request) {
	s.dataFeedHandler(w, r)
}

func (p *DebugPlugin) Grf(w http.ResponseWriter, r *http.Request) {
	dConf := debugger.Config{
		AttachPid:             os.Getpid(),
		Backend:               "default",
		CoreFile:              "",
		DebugInfoDirectories:  conf.DebugInfoDirectories,
		AttachWaitFor:         "",
		AttachWaitForInterval: 1,
		AttachWaitForDuration: 0,
	}
	dbg, err := debugger.New(&dConf, nil)
	defer dbg.Detach(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = myproc.ObjectReference(dbg.Target(), p.Grfout); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Write([]byte("ok"))
}

func (p *DebugPlugin) GetHeap(ctx context.Context, empty *emptypb.Empty) (*pb.HeapResponse, error) {
	// 创建临时文件用于存储堆信息
	f, err := os.CreateTemp("", "heap")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// 获取堆信息
	runtime.GC()
	if err := runtimePPROF.WriteHeapProfile(f); err != nil {
		return nil, err
	}

	// 读取堆信息
	f.Seek(0, 0)
	prof, err := profile.Parse(f)
	if err != nil {
		return nil, err
	}

	// 准备响应数据
	resp := &pb.HeapResponse{
		Data: &pb.HeapData{
			Stats:   &pb.HeapStats{},
			Objects: make([]*pb.HeapObject, 0),
			Edges:   make([]*pb.HeapEdge, 0),
		},
	}

	// 创建类型映射用于聚合统计
	typeMap := make(map[string]*pb.HeapObject)
	var totalSize int64

	// 处理每个样本
	for _, sample := range prof.Sample {
		size := sample.Value[1] // 内存大小
		if size == 0 {
			continue
		}

		// 获取分配类型信息
		var typeName string
		if len(sample.Location) > 0 && len(sample.Location[0].Line) > 0 {
			if fn := sample.Location[0].Line[0].Function; fn != nil {
				typeName = fn.Name
			}
		}

		// 创建或更新堆对象
		obj, exists := typeMap[typeName]
		if !exists {
			obj = &pb.HeapObject{
				Type:    typeName,
				Address: fmt.Sprintf("%p", sample),
				Refs:    make([]string, 0),
			}
			typeMap[typeName] = obj
			resp.Data.Objects = append(resp.Data.Objects, obj)
		}

		obj.Count++
		obj.Size += size
		totalSize += size

		// 构建引用关系
		for i := 1; i < len(sample.Location); i++ {
			loc := sample.Location[i]
			if len(loc.Line) == 0 || loc.Line[0].Function == nil {
				continue
			}

			callerName := loc.Line[0].Function.Name
			// 跳过系统函数
			if callerName == "" || strings.HasPrefix(callerName, "runtime.") {
				continue
			}

			// 添加边
			edge := &pb.HeapEdge{
				From:      callerName,
				To:        typeName,
				FieldName: callerName,
			}
			resp.Data.Edges = append(resp.Data.Edges, edge)

			// 将调用者添加到引用列表
			if !contains(obj.Refs, callerName) {
				obj.Refs = append(obj.Refs, callerName)
			}
		}
	}

	// 计算百分比
	for _, obj := range resp.Data.Objects {
		if totalSize > 0 {
			obj.SizePerc = float64(obj.Size) / float64(totalSize) * 100
		}
	}

	// 按大小排序
	sort.Slice(resp.Data.Objects, func(i, j int) bool {
		return resp.Data.Objects[i].Size > resp.Data.Objects[j].Size
	})

	// 获取运行时内存统计
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	// 填充内存统计信息
	resp.Data.Stats.Alloc = ms.Alloc
	resp.Data.Stats.TotalAlloc = ms.TotalAlloc
	resp.Data.Stats.Sys = ms.Sys
	resp.Data.Stats.NumGC = ms.NumGC
	resp.Data.Stats.HeapAlloc = ms.HeapAlloc
	resp.Data.Stats.HeapSys = ms.HeapSys
	resp.Data.Stats.HeapIdle = ms.HeapIdle
	resp.Data.Stats.HeapInuse = ms.HeapInuse
	resp.Data.Stats.HeapReleased = ms.HeapReleased
	resp.Data.Stats.HeapObjects = ms.HeapObjects
	resp.Data.Stats.GcCPUFraction = ms.GCCPUFraction

	return resp, nil
}

func (p *DebugPlugin) GetCpu(ctx context.Context, empty *emptypb.Empty) (*pb.CpuResponse, error) {
	// 创建一个临时文件来存储CPU profile数据
	f, err := os.CreateTemp("", "cpu_profile")
	if err != nil {
		return nil, fmt.Errorf("could not create CPU profile: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// 开始CPU profiling
	if err := runtimePPROF.StartCPUProfile(f); err != nil {
		return nil, fmt.Errorf("could not start CPU profile: %v", err)
	}

	// 采样指定时间
	time.Sleep(1 * time.Second)
	runtimePPROF.StopCPUProfile()

	// 读取profile数据
	profileData, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("could not read CPU profile: %v", err)
	}

	// 解析profile数据
	parsedProfile, err := profile.Parse(bytes.NewReader(profileData))
	if err != nil {
		return nil, fmt.Errorf("could not parse CPU profile: %v", err)
	}

	// 获取阻塞分析数据
	blockBuf := &bytes.Buffer{}
	if err := runtimePPROF.Lookup("block").WriteTo(blockBuf, 1); err != nil {
		return nil, fmt.Errorf("could not write block profile: %v", err)
	}

	// 解析阻塞分析数据
	blockProfile, err := profile.Parse(bytes.NewReader(blockBuf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("could not parse block profile: %v", err)
	}

	// 计算总阻塞时间
	var totalBlockingTime uint64
	for _, sample := range blockProfile.Sample {
		totalBlockingTime += uint64(sample.Value[0])
	}

	// 构建响应
	resp := &pb.CpuResponse{
		Data: &pb.CpuData{
			TotalCpuTimeNs:     uint64(parsedProfile.DurationNanos),
			SamplingIntervalNs: uint64(parsedProfile.Period),
			Functions:          make([]*pb.FunctionProfile, 0),
			Goroutines:         make([]*pb.GoroutineProfile, 0),
			SystemCalls:        make([]*pb.SystemCall, 0),
			RuntimeStats:       &pb.RuntimeStats{},
		},
	}

	// 收集函数调用信息
	for _, sample := range parsedProfile.Sample {
		functionProfile := &pb.FunctionProfile{
			FunctionName:    sample.Location[0].Line[0].Function.Name,
			CpuTimeNs:       uint64(sample.Value[0]),
			InvocationCount: uint64(sample.Value[1]),
			CallStack:       make([]string, 0),
		}

		// 收集调用栈信息
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				functionProfile.CallStack = append(functionProfile.CallStack, line.Function.Name)
			}
		}

		resp.Data.Functions = append(resp.Data.Functions, functionProfile)
	}

	// 收集运行时统计信息
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	resp.Data.RuntimeStats = &pb.RuntimeStats{
		GcCpuFraction:  memStats.GCCPUFraction,
		GcCount:        uint64(memStats.NumGC),
		GcPauseTimeNs:  memStats.PauseTotalNs,
		BlockingTimeNs: totalBlockingTime, // 添加阻塞时间统计
	}

	return resp, nil
}

// 辅助函数：检查字符串切片是否包含特定字符串
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func (p *DebugPlugin) GetHeapGraph(ctx context.Context, empty *emptypb.Empty) (*pb.HeapGraphResponse, error) {
	// 创建临时文件用于存储堆信息
	f, err := os.CreateTemp("", "heap")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// 获取堆信息
	runtime.GC()
	if err := runtimePPROF.WriteHeapProfile(f); err != nil {
		return nil, err
	}

	// 读取堆信息
	f.Seek(0, 0)
	profile, err := profile.Parse(f)
	if err != nil {
		return nil, err
	}
	// Generate dot graph.
	dot, err := debug.GetDotGraph(profile)
	if err != nil {
		return nil, err
	}
	return &pb.HeapGraphResponse{
		Data: dot,
	}, nil
}

func (p *DebugPlugin) GetCpuGraph(ctx context.Context, empty *emptypb.Empty) (*pb.CpuGraphResponse, error) {
	// 创建临时文件用于存储CPU profile信息
	f, err := os.CreateTemp("", "cpu")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// 开始CPU profile收集
	if err := runtimePPROF.StartCPUProfile(f); err != nil {
		return nil, err
	}

	// 收集30秒的CPU数据
	time.Sleep(1 * time.Second)
	runtimePPROF.StopCPUProfile()

	// 读取CPU profile信息
	f.Seek(0, 0)
	profile, err := profile.Parse(f)
	if err != nil {
		return nil, err
	}

	// 生成dot图
	dot, err := debug.GetDotGraph(profile)
	if err != nil {
		return nil, err
	}
	return &pb.CpuGraphResponse{
		Data: dot,
	}, nil
}
