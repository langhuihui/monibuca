package ffmpeg

import (
	"fmt"
	"os/exec"
	"strings"

	task "github.com/langhuihui/gotask"
)

type FFmpegTask struct {
	task.Task
	process *FFmpegProcess
	cmd     *exec.Cmd
}

func NewFFmpegTask(process *FFmpegProcess) *FFmpegTask {
	t := &FFmpegTask{
		process: process,
	}
	return t
}

func (t *FFmpegTask) GetKey() uint {
	return t.process.ID
}

func (t *FFmpegTask) Start() error {
	// 解析参数
	args := []string{}
	if t.process.Arguments != "" {
		// 这里应该解析JSON格式的参数
		// 为简化实现，我们直接将Arguments作为参数传递
		args = append(args, t.process.Arguments)
	}

	// 创建命令
	t.cmd = exec.Command("ffmpeg", args...)

	// 启动进程
	err := t.cmd.Start()
	if err != nil {
		t.process.Status = "error"
		return err
	}

	// 更新进程信息
	t.process.PID = t.cmd.Process.Pid
	t.process.Status = "running"
	t.OnStop(t.cmd.Process.Kill)
	return nil
}

func (t *FFmpegTask) Run() error {
	if t.cmd == nil || t.cmd.Process == nil {
		return fmt.Errorf("FFmpeg process not started")
	}

	// 等待进程结束
	err := t.cmd.Wait()
	if err != nil {
		t.process.Status = "error"
		return err
	}
	t.process.Status = "stopped"
	return nil
}

// getFFmpegVersion 获取 FFmpeg 版本信息
func getFFmpegVersion() (version, buildInfo string, err error) {
	// 执行 ffmpeg -version 命令
	cmd := exec.Command("ffmpeg", "-version")
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to execute ffmpeg -version: %v", err)
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	if len(lines) == 0 {
		return "", "", fmt.Errorf("no output from ffmpeg -version")
	}

	// 第一行通常包含版本信息，格式类似：ffmpeg version 4.4.2
	firstLine := strings.TrimSpace(lines[0])

	// 提取版本号
	parts := strings.Fields(firstLine)
	var versionPart string
	for i, part := range parts {
		if part == "version" && i+1 < len(parts) {
			versionPart = parts[i+1]
			break
		}
	}

	if versionPart == "" {
		versionPart = firstLine
	}

	// 构建信息包含所有输出行
	buildInfo = strings.TrimSpace(outputStr)

	return versionPart, buildInfo, nil
}
