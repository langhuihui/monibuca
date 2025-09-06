package ffmpeg

import (
	task "github.com/langhuihui/gotask"
	"m7s.live/v5"
	"m7s.live/v5/plugin/ffmpeg/pb"
)

var _ = m7s.InstallPlugin[FFmpegPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

var _ pb.ApiServer = (*FFmpegPlugin)(nil)

type FFmpegPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	processes task.WorkCollection[uint, *FFmpegTask]
}

func (p *FFmpegPlugin) Start() error {
	if p.DB != nil {
		// 注册数据模型
		p.DB.AutoMigrate(&FFmpegProcess{})

		// 启动时加载自动启动的进程
		var processes []*FFmpegProcess
		p.DB.Where("auto_start = ?", true).Find(&processes)

		for _, process := range processes {
			p.startProcess(process)
		}
	}
	version, _, err := getFFmpegVersion()
	if err != nil {
		return err
	}
	p.Info("Found FFmpeg", "version", version)
	return nil
}
