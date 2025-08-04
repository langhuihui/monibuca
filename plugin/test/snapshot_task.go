package plugin_test

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/format"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

var ffmpegArgs = []string{
	"-hide_banner",
	"-i", "pipe:0",
	"-vframes", "1",
	"-f", "mjpeg", "pipe:1",
}

type SnapshotTask struct {
	TestBaseTask
}

func (st *SnapshotTask) Run() error {
	var cmd *exec.Cmd
	streamPath := st.StreamPath
	if st.Format == "" {
		// 创建订阅配置
		subConfig := st.testCase.Plugin.GetCommonConf().Subscribe
		subConfig.SubType = m7s.SubscribeTypeTransform
		subConfig.IFrameOnly = true

		subscriber, err := st.testCase.Plugin.SubscribeWithConfig(st, streamPath, subConfig)
		if err != nil {
			return fmt.Errorf("failed to subscribe to stream: %w", err)
		}
		var annexB *format.AnnexB
		track := subscriber.Publisher.GetVideoTrack(reflect.TypeOf(annexB))
		track.WaitReady()
		reader := pkg.NewAVRingReader(track, "annexb")
		err = reader.ReadFrame(&subConfig)
		if err != nil {
			return fmt.Errorf("failed to read frame: %w", err)
		}
		annexB = reader.Value.Wraps[track.WrapIndex].(*format.AnnexB)
		var mem util.Memory
		mem.CopyFrom(&annexB.Memory)
		reader.StopRead()
		cmd = exec.CommandContext(st, "ffmpeg", ffmpegArgs...)
		r := mem.NewReader()
		cmd.Stdin = &r
	} else {
		cmd = exec.CommandContext(st, "ffmpeg", "-hide_banner", "-i", st.GetInput(streamPath), "-vframes", "1", "-f", "mjpeg", "pipe:1")
	}
	var buf util.Buffer
	cmd.Stderr = st
	cmd.Stdout = &buf
	st.Info("starting ffmpeg", "args", strings.Join(cmd.Args, " "))
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	st.OnStop(cmd.Process.Kill)
	cmd.Wait()
	if buf.Len() == 0 {
		return fmt.Errorf("snapshot output is empty")
	}
	st.Info("Snapshot completed successfully", "outputSize", buf.Len())
	return task.ErrTaskComplete
}

func (st *SnapshotTask) GetInput(streamPath string) string {
	switch st.Format {
	case "rtmp", "rtsp":
		return fmt.Sprintf("%s://%s/%s", st.Format, st.ServerAddr, streamPath)
	case "srt":
		return fmt.Sprintf("srt://%s?streamid=subscribe:/%s", st.ServerAddr, streamPath)
	case "flv":
		return fmt.Sprintf("http://%s/flv/%s.flv", st.ServerAddr, streamPath)
	case "mp4":
		return fmt.Sprintf("http://%s/mp4/%s.mp4", st.ServerAddr, streamPath)
	}
	return ""
}

func (st *SnapshotTask) Write(buf []byte) (int, error) {
	return st.testCase.Write(append([]byte("[snapshot]"), buf...))
}

func init() {
	testTaskFactory.Register("snapshot", func(s *TestCase, conf TestTaskConfig) task.ITask {
		return &SnapshotTask{TestBaseTask: TestBaseTask{testCase: s, TestTaskConfig: conf}}
	})
}
