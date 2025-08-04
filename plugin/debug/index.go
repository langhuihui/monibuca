package plugin_debug

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec" // 新增导入
	"runtime"
	runtimePPROF "runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
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

var _ = m7s.InstallPlugin[DebugPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})
var conf, _ = config.LoadConfig()

type DebugPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	ProfileDuration time.Duration `default:"10s" desc:"profile持续时间"`
	Profile         string        `desc:"采集profile存储文件"`
	Grfout          string        `default:"grf.out" desc:"grf输出文件"`
	EnableChart     bool          `default:"true" desc:"是否启用图表功能"`
	// 添加缓存字段
	cpuProfileData *profile.Profile // 缓存 CPU Profile 数据
	cpuProfileOnce sync.Once        // 确保只采集一次
	cpuProfileLock sync.Mutex       // 保护缓存数据
	chartServer    server
}

type WriteToFile struct {
	header http.Header
	io.Writer
}

func (w *WriteToFile) Header() http.Header {
	return w.header
}

func (w *WriteToFile) WriteHeader(statusCode int) {}

func (p *DebugPlugin) Start() error {
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
	if p.EnableChart {
		p.AddTask(&p.chartServer)
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
	p.chartServer.dataHandler(w, r)
}

func (p *DebugPlugin) Charts_datafeed(w http.ResponseWriter, r *http.Request) {
	p.chartServer.dataFeedHandler(w, r)
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

// 采集 CPU Profile 并缓存
func (p *DebugPlugin) collectCPUProfile() error {
	p.cpuProfileLock.Lock()
	defer p.cpuProfileLock.Unlock()

	// 如果已经采集过，直接返回
	if p.cpuProfileData != nil {
		return nil
	}

	// 创建临时文件用于存储 CPU Profile 数据
	f, err := os.CreateTemp("", "cpu_profile")
	if err != nil {
		return fmt.Errorf("could not create CPU profile: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	// 开始 CPU profiling
	if err := runtimePPROF.StartCPUProfile(f); err != nil {
		return fmt.Errorf("could not start CPU profile: %v", err)
	}

	// 采样指定时间
	time.Sleep(p.ProfileDuration)
	runtimePPROF.StopCPUProfile()

	// 读取并解析 CPU Profile 数据
	f.Seek(0, 0)
	profileData, err := profile.Parse(f)
	if err != nil {
		return fmt.Errorf("could not parse CPU profile: %v", err)
	}

	// 缓存 CPU Profile 数据
	p.cpuProfileData = profileData
	return nil
}

// GetCpu 接口
func (p *DebugPlugin) GetCpu(ctx context.Context, req *pb.CpuRequest) (*pb.CpuResponse, error) {
	// 如果需要刷新或者缓存中没有数据
	if req.Refresh || p.cpuProfileData == nil {
		p.cpuProfileLock.Lock()
		p.cpuProfileData = nil         // 清除现有缓存
		p.cpuProfileOnce = sync.Once{} // 重置 Once
		p.cpuProfileLock.Unlock()
	}

	// 如果请求指定了duration，临时更新ProfileDuration
	originalDuration := p.ProfileDuration
	if req.Duration > 0 {
		p.ProfileDuration = time.Duration(req.Duration) * time.Second
	}

	// 确保采集 CPU Profile
	p.cpuProfileOnce.Do(func() {
		if err := p.collectCPUProfile(); err != nil {
			fmt.Printf("Failed to collect CPU profile: %v\n", err)
		}
	})

	// 恢复原始的ProfileDuration
	if req.Duration > 0 {
		p.ProfileDuration = originalDuration
	}

	// 如果缓存中没有数据，返回错误
	if p.cpuProfileData == nil {
		return nil, fmt.Errorf("CPU profile data is not available")
	}

	// 使用缓存的 CPU Profile 数据构建响应
	resp := &pb.CpuResponse{
		Data: &pb.CpuData{
			TotalCpuTimeNs:     uint64(p.cpuProfileData.DurationNanos),
			SamplingIntervalNs: uint64(p.cpuProfileData.Period),
			Functions:          make([]*pb.FunctionProfile, 0),
			Goroutines:         make([]*pb.GoroutineProfile, 0),
			SystemCalls:        make([]*pb.SystemCall, 0),
			RuntimeStats:       &pb.RuntimeStats{},
		},
	}

	// 填充函数调用信息
	for _, sample := range p.cpuProfileData.Sample {
		functionProfile := &pb.FunctionProfile{
			FunctionName:    sample.Location[0].Line[0].Function.Name,
			CpuTimeNs:       uint64(sample.Value[0]),
			InvocationCount: uint64(sample.Value[1]),
			CallStack:       make([]string, 0),
		}

		// 填充调用栈信息
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				functionProfile.CallStack = append(functionProfile.CallStack, line.Function.Name)
			}
		}

		resp.Data.Functions = append(resp.Data.Functions, functionProfile)
	}

	return resp, nil
}

// GetCpuGraph 接口
func (p *DebugPlugin) GetCpuGraph(ctx context.Context, req *pb.CpuRequest) (*pb.CpuGraphResponse, error) {
	// 如果需要刷新或者缓存中没有数据
	if req.Refresh || p.cpuProfileData == nil {
		p.cpuProfileLock.Lock()
		p.cpuProfileData = nil         // 清除现有缓存
		p.cpuProfileOnce = sync.Once{} // 重置 Once
		p.cpuProfileLock.Unlock()
	}

	// 如果请求指定了duration，临时更新ProfileDuration
	originalDuration := p.ProfileDuration
	if req.Duration > 0 {
		p.ProfileDuration = time.Duration(req.Duration) * time.Second
	}

	// 确保采集 CPU Profile
	p.cpuProfileOnce.Do(func() {
		if err := p.collectCPUProfile(); err != nil {
			fmt.Printf("Failed to collect CPU profile: %v\n", err)
		}
	})

	// 恢复原始的ProfileDuration
	if req.Duration > 0 {
		p.ProfileDuration = originalDuration
	}

	// 如果缓存中没有数据，返回错误
	if p.cpuProfileData == nil {
		return nil, fmt.Errorf("CPU profile data is not available")
	}

	// 使用缓存的 CPU Profile 数据生成 dot 图
	dot, err := debug.GetDotGraph(p.cpuProfileData)
	if err != nil {
		return nil, fmt.Errorf("could not generate dot graph: %v", err)
	}

	return &pb.CpuGraphResponse{
		Data: dot,
	}, nil
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

func (p *DebugPlugin) API_TcpDump(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	args := []string{"-S", "tcpdump", "-w", "dump.cap"}
	if query.Get("interface") != "" {
		args = append(args, "-i", query.Get("interface"))
	}
	if query.Get("filter") != "" {
		args = append(args, query.Get("filter"))
	}
	if query.Get("extra_args") != "" {
		args = append(args, strings.Fields(query.Get("extra_args"))...)
	}
	if query.Get("duration") == "" {
		http.Error(rw, "duration is required", http.StatusBadRequest)
		return
	}
	// rw.Header().Set("Content-Type", "text/plain")
	// rw.Header().Set("Cache-Control", "no-cache")
	// rw.Header().Set("Content-Disposition", "attachment; filename=tcpdump.txt")
	duration, err := strconv.Atoi(query.Get("duration"))
	if err != nil {
		http.Error(rw, "invalid duration", http.StatusBadRequest)
		return
	}
	ctx, _ := context.WithTimeout(p, time.Duration(duration)*time.Second)
	cmd := exec.CommandContext(ctx, "sudo", args...)
	p.Info("starting tcpdump", "args", strings.Join(cmd.Args, " "))
	cmd.Stdin = strings.NewReader(query.Get("password"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr // 将错误输出重定向到标准错误
	err = cmd.Start()
	if err != nil {
		http.Error(rw, fmt.Sprintf("failed to start tcpdump: %v", err), http.StatusInternalServerError)
		return
	}
	<-ctx.Done()
	killcmd := exec.Command("sudo", "-S", "pkill", "-9", "tcpdump")
	p.Info("killing tcpdump", "args", strings.Join(killcmd.Args, " "))
	killcmd.Stdin = strings.NewReader(query.Get("password"))
	killcmd.Stderr = os.Stderr
	killcmd.Stdout = os.Stdout
	killcmd.Run()
	p.Info("kill done")
	cmd.Wait()
	p.Info("dump done")
	http.ServeFile(rw, r, "dump.cap")
}
