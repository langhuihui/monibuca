package plugin_test

import (
	"fmt"

	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	flv "m7s.live/v5/plugin/flv/pkg"
	hls "m7s.live/v5/plugin/hls/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
	rtsp "m7s.live/v5/plugin/rtsp/pkg"
	srt "m7s.live/v5/plugin/srt/pkg"
	webrtc "m7s.live/v5/plugin/webrtc/pkg"
)

func init() {
	testTaskFactory.Register("read", func(s *TestCase, conf TestTaskConfig) task.ITask {
		return &ReadRemoteTask{TestBaseTask: TestBaseTask{testCase: s, TestTaskConfig: conf}}
	})
}

type ReadRemoteTask struct {
	TestBaseTask
}

func (mt *ReadRemoteTask) Start() error {
	var conf config.Pull
	conf.URL = mt.Input
	streamPath := mt.StreamPath
	var puller m7s.IPuller
	switch mt.Format {
	case "mp4":
		if conf.URL == "" {
			conf.URL = "test.mp4"
		}
		puller = mp4.NewPuller(conf)
	case "flv":
		if conf.URL == "" {
			conf.URL = "test.flv"
		}
		puller = flv.NewPuller(conf)
	case "annexb":
		conf.URL = fmt.Sprintf("http://%s:8080/annexb/%s", mt.ServerAddr, mt.Input)
		puller = m7s.NewAnnexBPuller(conf)
	case "rtmp":
		puller = rtmp.NewPuller(conf)
	case "rtsp":
		puller = rtsp.NewPuller(conf)
	case "srt":
		puller = srt.NewPuller(conf)
	case "hls":
		puller = hls.NewPuller(conf)
	case "webrtc":
		conf.URL = fmt.Sprintf("http://%s:8080/webrtc/play/%s", mt.ServerAddr, mt.Input)
		puller = webrtc.NewPuller(conf)
	}
	pulljob := puller.GetPullJob().Init(puller, &mt.testCase.Plugin.Plugin, streamPath, conf, nil)
	mt.Using(pulljob)
	pulljob.Using(mt)
	return nil
}
