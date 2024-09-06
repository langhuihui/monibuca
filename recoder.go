package m7s

import (
	"m7s.live/m7s/v5/pkg/config"
	"os"
	"path/filepath"
	"time"

	"m7s.live/m7s/v5/pkg/task"

	"m7s.live/m7s/v5/pkg"
)

type (
	IRecorder interface {
		task.ITask
		GetRecordJob() *RecordJob
	}
	Recorder  = func() IRecorder
	RecordJob struct {
		task.Job
		StreamPath string // 对应本地流
		Plugin     *Plugin
		Subscriber *Subscriber
		Fragment   time.Duration
		Append     bool
		FilePath   string
		recorder   IRecorder
	}
	DefaultRecorder struct {
		task.Task
		RecordJob RecordJob
	}
)

func (r *DefaultRecorder) GetRecordJob() *RecordJob {
	return &r.RecordJob
}

func (r *DefaultRecorder) Start() (err error) {
	return r.RecordJob.Subscribe()
}

func (p *RecordJob) GetKey() string {
	return p.FilePath
}

func (p *RecordJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.recorder.GetTask().Context, p.StreamPath)
	return
}

func (p *RecordJob) Init(recorder IRecorder, plugin *Plugin, streamPath string, conf config.Record) *RecordJob {
	p.Plugin = plugin
	p.Fragment = conf.Fragment
	p.Append = conf.Append
	p.FilePath = conf.FilePath
	p.StreamPath = streamPath
	p.recorder = recorder
	p.Description = map[string]any{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"filePath":   conf.FilePath,
		"append":     conf.Append,
		"fragment":   conf.Fragment,
	}
	plugin.Server.Records.Add(p, plugin.Logger.With("filePath", conf.FilePath, "streamPath", streamPath))
	return p
}

func (p *RecordJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Records.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	dir := p.FilePath
	if p.Fragment == 0 || p.Append {
		dir = filepath.Dir(p.FilePath)
	}
	p.Description["filePath"] = p.FilePath
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	p.AddTask(p.recorder, p.Logger)
	return
}
