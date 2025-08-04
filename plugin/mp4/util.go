package plugin_mp4

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
)

// ProcessWithFFmpeg 使用 FFmpeg 处理视频帧并生成截图
func ProcessWithFFmpeg(data net.Buffers, index int, output io.Writer) error {
	// 创建ffmpeg命令，直接输出JPEG格式
	cmd := exec.Command("ffmpeg",
		"-hide_banner",
		"-i", "pipe:0",
		"-vf", fmt.Sprintf("select=eq(n\\,%d)", index),
		"-vframes", "1",
		"-f", "mjpeg",
		"pipe:1")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		errOutput, _ := io.ReadAll(stderr)
		log.Printf("FFmpeg stderr: %s", errOutput)
	}()

	if err = cmd.Start(); err != nil {
		log.Printf("cmd.Start失败: %v", err)
		return err
	}

	go func() {
		defer stdin.Close()
		data.WriteTo(stdin)
	}()

	// 从ffmpeg的stdout读取JPEG数据并写入到输出
	if _, err = io.Copy(output, stdout); err != nil {
		log.Printf("读取失败: %v", err)
		return err
	}
	if err = cmd.Wait(); err != nil {
		log.Printf("cmd.Wait失败: %v", err)
		return err
	}

	log.Printf("ffmpeg JPEG输出成功")
	return nil
}
