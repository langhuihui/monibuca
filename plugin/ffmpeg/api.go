package ffmpeg

import (
	"context"

	task "github.com/langhuihui/gotask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	globalPb "m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/plugin/ffmpeg/pb"
)

// CreateProcess 创建 FFmpeg 进程
func (p *FFmpegPlugin) CreateProcess(ctx context.Context, req *pb.CreateProcessRequest) (*pb.ProcessResponse, error) {
	process := &FFmpegProcess{
		Description: req.Description,
		Arguments:   req.Arguments,
		AutoStart:   req.AutoStart,
		Status:      "stopped",
	}

	// 保存到数据库
	if err := p.DB.Create(process).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to create process: %v", err)
	}

	// 如果设置了自动启动，则启动进程
	if process.AutoStart {
		p.startProcess(process)
	}

	return &pb.ProcessResponse{
		Code:    0,
		Message: "success",
		Data: &pb.FFmpegProcess{
			Id:          uint32(process.ID),
			Description: process.Description,
			Arguments:   process.Arguments,
			Status:      process.Status,
			Pid:         int32(process.PID),
			AutoStart:   process.AutoStart,
			CreatedAt:   timestamppb.New(process.CreatedAt),
			UpdatedAt:   timestamppb.New(process.UpdatedAt),
		},
	}, nil
}

// UpdateProcess 更新 FFmpeg 进程
func (p *FFmpegPlugin) UpdateProcess(ctx context.Context, req *pb.UpdateProcessRequest) (*pb.ProcessResponse, error) {
	process := &FFmpegProcess{}
	if err := p.DB.First(process, req.Id).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "Process not found: %v", err)
	}

	process.Description = req.Description
	process.Arguments = req.Arguments
	process.AutoStart = req.AutoStart

	// 更新数据库
	if err := p.DB.Save(process).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to update process: %v", err)
	}

	return &pb.ProcessResponse{
		Code:    0,
		Message: "success",
		Data: &pb.FFmpegProcess{
			Id:          uint32(process.ID),
			Description: process.Description,
			Arguments:   process.Arguments,
			Status:      process.Status,
			Pid:         int32(process.PID),
			AutoStart:   process.AutoStart,
			CreatedAt:   timestamppb.New(process.CreatedAt),
			UpdatedAt:   timestamppb.New(process.UpdatedAt),
		},
	}, nil
}

// DeleteProcess 删除 FFmpeg 进程
func (p *FFmpegPlugin) DeleteProcess(ctx context.Context, req *pb.ProcessIdRequest) (*globalPb.SuccessResponse, error) {
	process := &FFmpegProcess{}
	if err := p.DB.First(process, req.Id).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "Process not found: %v", err)
	}
	if process, exists := p.processes.Get(uint(req.Id)); exists {
		process.Stop(task.ErrStopByUser)
		process.WaitStopped()
	}
	if err := p.DB.Delete(process).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to delete process: %v", err)
	}
	return &globalPb.SuccessResponse{
		Code:    0,
		Message: "success",
	}, nil
}

// ListProcesses 获取 FFmpeg 进程列表
func (p *FFmpegPlugin) ListProcesses(ctx context.Context, req *emptypb.Empty) (*pb.ListProcessesResponse, error) {
	var processes []FFmpegProcess

	query := p.DB.Model(&FFmpegProcess{})

	if err := query.Find(&processes).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to list processes: %v", err)
	}

	data := make([]*pb.FFmpegProcess, len(processes))
	for i, process := range processes {
		runtimeProcess, ok := p.processes.Get(process.ID)
		if ok {
			process = *runtimeProcess.process
		}
		data[i] = &pb.FFmpegProcess{
			Id:          uint32(process.ID),
			Description: process.Description,
			Arguments:   process.Arguments,
			AutoStart:   process.AutoStart,
			Status:      process.Status,
			Pid:         int32(process.PID),
			CreatedAt:   timestamppb.New(process.CreatedAt),
			UpdatedAt:   timestamppb.New(process.UpdatedAt),
		}
	}

	return &pb.ListProcessesResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	}, nil
}

// StartProcess 启动 FFmpeg 进程
func (p *FFmpegPlugin) StartProcess(ctx context.Context, req *pb.ProcessIdRequest) (*globalPb.SuccessResponse, error) {
	process := &FFmpegProcess{}
	if err := p.DB.First(process, req.Id).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "Process not found: %v", err)
	}

	// 启动进程
	if err := p.startProcess(process); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to start process: %v", err)
	}

	return &globalPb.SuccessResponse{
		Code:    0,
		Message: "success",
	}, nil
}

// StopProcess 停止 FFmpeg 进程
func (p *FFmpegPlugin) StopProcess(ctx context.Context, req *pb.ProcessIdRequest) (*globalPb.SuccessResponse, error) {
	// 停止进程
	if process, exists := p.processes.Get(uint(req.Id)); exists {
		process.Stop(task.ErrStopByUser)
		process.WaitStopped()
	} else {
		return nil, pkg.ErrNotFound
	}
	return &globalPb.SuccessResponse{
		Code:    0,
		Message: "success",
	}, nil
}

// RestartProcess 重启 FFmpeg 进程
func (p *FFmpegPlugin) RestartProcess(ctx context.Context, req *pb.ProcessIdRequest) (*globalPb.SuccessResponse, error) {
	process := &FFmpegProcess{}
	if err := p.DB.First(process, req.Id).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "Process not found: %v", err)
	}

	// 如果进程正在运行，先停止它
	if task, exists := p.processes.Get(uint(req.Id)); exists {
		task.Stop(pkg.ErrRestart)
		task.WaitStopped()
	}

	err := p.startProcess(process).WaitStarted()
	if err != nil {
		return nil, err
	}
	return &globalPb.SuccessResponse{
		Code:    0,
		Message: "success",
	}, nil
}

// GetVersion 获取 FFmpeg 版本信息
func (p *FFmpegPlugin) GetVersion(ctx context.Context, req *emptypb.Empty) (*pb.VersionResponse, error) {
	version, _, err := getFFmpegVersion()
	if err != nil {
		return &pb.VersionResponse{
			Code:    1,
			Message: "Failed to get FFmpeg version: " + err.Error(),
		}, nil
	}

	return &pb.VersionResponse{
		Code:    0,
		Message: "success",
		Data:    version,
	}, nil
}

// startProcess 启动进程的内部方法
func (p *FFmpegPlugin) startProcess(process *FFmpegProcess) *task.Task {
	return p.AddTask(NewFFmpegTask(process))
}
