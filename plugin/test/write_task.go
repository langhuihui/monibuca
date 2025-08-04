package plugin_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
	hls "m7s.live/v5/plugin/hls/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/v5/plugin/rtsp/pkg"
	srt "m7s.live/v5/plugin/srt/pkg"
)

func init() {
	testTaskFactory.Register("write", func(s *TestCase, conf TestTaskConfig) task.ITask {
		return &WriteRemoteTask{TestBaseTask: TestBaseTask{testCase: s, TestTaskConfig: conf}}
	})
}

const RecordPath = "test_record"

type WriteRemoteTask struct {
	TestBaseTask
}

func (mt *WriteRemoteTask) Start() (err error) {
	var pushConf config.Push
	var recConf config.Record
	recConf.Mode = config.RecordModeTest
	// recConf.Fragment = time.Second * 10
	recConf.FilePath = RecordPath
	pushConf.URL = mt.Input
	var pusher m7s.IPusher
	var recorder m7s.IRecorder
	switch mt.Format {
	case "rtmp":
		pusher = rtmp.NewPusher()
	case "rtsp":
		pusher = rtsp.NewPusher()
	case "srt":
		pusher = srt.NewPusher()
	case "mp4":
		recorder = mp4.NewRecorder(recConf)
	case "flv":
		recorder = flv.NewRecorder(recConf)
	case "hls":
		recorder = hls.NewRecorder(recConf)
	case "ps":
	}
	if recorder != nil {
		// 清理录制文件目录
		if err := os.RemoveAll(RecordPath); err != nil {
			mt.testCase.Error("failed to clear record files:", err)
		}
		if err := os.MkdirAll(RecordPath, 0755); err != nil {
			mt.testCase.Error("failed to create record directory:", err)
		}
		recordJob := recorder.GetRecordJob().Init(recorder, &mt.testCase.Plugin.Plugin, mt.StreamPath, recConf, nil)
		mt.Using(recordJob)
		recordJob.Using(mt)
		time.AfterFunc(time.Second*10, func() {
			recordJob.Stop(task.ErrTaskComplete)
		})
	} else if pusher != nil {
		pushjob := pusher.GetPushJob().Init(pusher, &mt.testCase.Plugin.Plugin, mt.StreamPath, pushConf, nil)
		mt.Using(pushjob)
		pushjob.Using(mt)
	}
	return
}

func (mt *WriteRemoteTask) Go() (err error) {
	switch mt.Format {
	case "rtmp", "rtsp":
		<-time.After(time.Second * 5)
	case "mp4", "flv", "hls":
		time.Sleep(time.Second * 15)
		files, err := os.ReadDir(RecordPath)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			filePath := filepath.Join(RecordPath, file.Name())
			cmd := exec.Command("ffmpeg", "-hide_banner", "-i", filePath, "-vframes", "1", "-f", "mjpeg", "pipe:1")
			var buf util.Buffer
			cmd.Stderr = mt.testCase
			cmd.Stdout = &buf
			_ = cmd.Run()
			if buf.Len() == 0 {
				return fmt.Errorf("snapshot output is empty")
			}
			os.Remove(filePath)
		}
	case "ps":
		host := mt.ServerAddr
		if !strings.Contains(host, ":") {
			host += ":8080"
		}
		body := strings.NewReader(`{"streamPath":"` + mt.StreamPath + `","ip":"localhost","port":50000}`)
		ctx, cancel := context.WithTimeout(mt, time.Second*5)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", "http://"+host+"/rtp/send/ps", body)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		mt.Info(res.Status, "code", res.StatusCode, "url", req.URL)
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("write ps file failed")
		}
		defer res.Body.Close()
	}
	return
}
