package m7s

import (
	"m7s.live/m7s/v5/pkg/util"
	"os"
	"path/filepath"
	"time"

	"m7s.live/m7s/v5/pkg"
)

type Recorder = func(*RecordContext) error

func createRecoder(p *Plugin, streamPath string, filePath string) (recorder *RecordContext) {
	recorder = &RecordContext{
		Plugin:     p,
		Fragment:   p.config.Record.Fragment,
		Append:     p.config.Record.Append,
		FilePath:   filePath,
		StreamPath: streamPath,
	}
	recorder.Name = "record"
	recorder.Logger = p.Logger.With("filePath", filePath, "streamPath", streamPath)
	return
}

type RecordContext struct {
	util.MarcoTask
	StreamPath string // 对应本地流
	Plugin     *Plugin
	Subscriber *Subscriber
	Fragment   time.Duration
	Append     bool
	FilePath   string
}

func (p *RecordContext) GetKey() string {
	return p.FilePath
}

func (p *RecordContext) Do(recorder Recorder) {
	p.AddCall(func(tmpTask *util.Task) (err error) {
		dir := p.FilePath
		if p.Fragment == 0 || p.Append {
			if filepath.Ext(p.FilePath) == "" {
				p.FilePath += ".flv"
			}
			dir = filepath.Dir(p.FilePath)
		}
		if err = os.MkdirAll(dir, 0755); err != nil {
			return
		}
		p.Subscriber, err = p.Plugin.Subscribe(tmpTask.Context, p.StreamPath)
		if err != nil {
			return
		}
		err = recorder(p)
		return
	}, nil)
}

func (p *RecordContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Records.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	s.Records.Add(p)
	if p.Plugin.Meta.Recorder != nil {
		p.Do(p.Plugin.Meta.Recorder)
	}
	return
}

func (p *RecordContext) Dispose() {
	p.Plugin.Server.Records.Remove(p)
}
