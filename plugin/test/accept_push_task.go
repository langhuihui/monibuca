package plugin_test

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"m7s.live/v5/pkg/task"
)

func init() {
	testTaskFactory.Register("push", func(s *TestCase, conf TestTaskConfig) task.ITask {
		return &AcceptPushTask{TestBaseTask: TestBaseTask{testCase: s, TestTaskConfig: conf}}
	})
}

// AcceptPushTask RTSP 推流任务
type AcceptPushTask struct {
	TestBaseTask
}

// Start 任务开始
func (rt *AcceptPushTask) Start() error {
	if rt.Input == "" {
		rt.Input = "test.mp4"
	}
	// 构建 FFmpeg 命令
	args := []string{
		"-hide_banner",
		"-re", // 以实时速率读取输入
		"-stream_loop", "-1", "-i", rt.Input,
	}
	// 添加视频编码参数
	if !rt.testCase.AudioOnly {
		args = append(args, "-c:v", rt.testCase.VideoCodec)
	} else {
		args = append(args, "-vn")
	}

	// 添加音频编码参数
	if !rt.testCase.VideoOnly {
		args = append(args, "-c:a", rt.testCase.AudioCodec)
	} else {
		args = append(args, "-an")
	}

	switch rt.Format {
	case "rtsp":
		args = append(args,
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			fmt.Sprintf("rtsp://%s/%s", rt.ServerAddr, rt.StreamPath),
		)
	case "rtmp":
		args = append(args,
			"-f", "flv",
			fmt.Sprintf("rtmp://%s/%s", rt.ServerAddr, rt.StreamPath),
		)
	case "srt":
		args = append(args, "-f", "mpegts", fmt.Sprintf("srt://%s?streamid=publish:/%s", rt.ServerAddr, rt.StreamPath))
	case "webrtc":
		args = append(args, "-f", "whip", fmt.Sprintf("http://%s:8080/webrtc/push/%s", rt.ServerAddr, rt.StreamPath))
	case "ps":
		host := rt.ServerAddr
		if !strings.Contains(host, ":") {
			host += ":8080"
		}
		body := strings.NewReader(`{"streamPath":"` + rt.StreamPath + `","port":50000}`)
		req, err := http.NewRequestWithContext(rt, "POST", "http://"+host+"/rtp/receive/ps", body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		go func() {
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				rt.Stop(err)
				return
			}
			rt.Info(res.Status, "code", res.StatusCode, "url", req.URL)
			if res.StatusCode != http.StatusOK {
				rt.Stop(errors.New("receive ps failed"))
				return
			}
			defer res.Body.Close()
		}()
	}

	rt.Info("FFmpeg command", "command", strings.Join(args, " "))

	// 创建进程
	cmd := exec.Command("ffmpeg", args...)
	// 日志重定向
	cmd.Stdout = rt.testCase
	cmd.Stderr = rt.testCase
	// 启动进程
	if err := cmd.Start(); err != nil {
		rt.testCase.Status = TestCaseStatusFailed
		return err
	}
	rt.OnStop(cmd.Process.Kill)
	return nil
}
