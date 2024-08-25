package m7s

import (
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

func (p *RecordJob) Init(recorder IRecorder, plugin *Plugin, streamPath string, filePath string) *RecordJob {
	p.Plugin = plugin
	p.Fragment = plugin.config.Record.Fragment
	p.Append = plugin.config.Record.Append
	p.FilePath = filePath
	p.StreamPath = streamPath
	p.Logger = plugin.Logger.With("filePath", filePath, "streamPath", streamPath)
	if recorderTask := recorder.GetTask(); recorderTask.Logger == nil {
		recorderTask.Logger = p.Logger
	}
	p.recorder = recorder
	plugin.Server.Records.Add(p)
	return p
}

func (p *RecordJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Records.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	dir := p.FilePath
	if p.Fragment == 0 || p.Append {
		if filepath.Ext(p.FilePath) == "" {
			p.FilePath += ".flv"
		}
		dir = filepath.Dir(p.FilePath)
	}
	p.Description = map[string]any{
		"filePath": p.FilePath,
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	p.AddTask(p.recorder)
	return
}
