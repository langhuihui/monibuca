package m7s

import (
	"context"
	"m7s.live/m7s/v5/pkg"
	"time"
)

type Recorder = func(*RecordContext) error

func createRecoder(p *Plugin, streamPath string, filePath string, options ...any) (recorder *RecordContext) {
	recorder = &RecordContext{
		Plugin:   p,
		Fragment: p.config.Record.Fragment,
		Append:   p.config.Record.Append,
		FilePath: filePath,
	}
	recorder.ID = p.Server.recordTask.GetID()
	recorder.FilePath = filePath
	recorder.SubscribeOptions = []any{p.config.Subscribe}
	var ctx = p.Context
	for _, option := range options {
		switch v := option.(type) {
		case context.Context:
			ctx = v
		default:
			recorder.SubscribeOptions = append(recorder.SubscribeOptions, option)
		}
	}
	recorder.Init(ctx, p.Logger.With("filePath", filePath, "streamPath", streamPath), recorder)
	recorder.SubscribeOptions = append(recorder.SubscribeOptions, recorder.Context)
	return
}

type RecordContext struct {
	pkg.Task
	Plugin           *Plugin
	Subscriber       *Subscriber
	SubscribeOptions []any
	Fragment         time.Duration
	Append           bool
	FilePath         string
}

func (p *RecordContext) GetKey() string {
	return p.FilePath
}

func (p *RecordContext) Run(recorder Recorder) {
	err := recorder(p)
	p.Stop(err)
}

func (p *RecordContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Records.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	s.Records.Add(p)
	return
}

func (p *RecordContext) Dispose() {
	p.Plugin.Server.Records.Remove(p)
}
