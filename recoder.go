package m7s

import (
	"m7s.live/m7s/v5/pkg/task"
	"os"
	"path/filepath"
	"time"

	"m7s.live/m7s/v5/pkg"
)

type (
	IRecorder interface {
		task.ITask
		GetRecordContext() *RecordContext
	}
	Recorder      = func() IRecorder
	RecordContext struct {
		task.MarcoTask
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
		Ctx RecordContext
	}
)

func (r *DefaultRecorder) GetRecordContext() *RecordContext {
	return &r.Ctx
}

func (r *DefaultRecorder) Start() (err error) {
	return r.Ctx.Subscribe()
}

func (p *RecordContext) GetKey() string {
	return p.FilePath
}

func (p *RecordContext) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.recorder.GetTask().Context, p.StreamPath)
	return
}

func (p *RecordContext) Init(recorder IRecorder, plugin *Plugin, streamPath string, filePath string) *RecordContext {
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

func (p *RecordContext) Start() (err error) {
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
