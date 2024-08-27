package m7s

import (
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
)

type (
	ITransformer interface {
		task.ITask
		GetTransformJob() *TransformJob
	}
	Transformer  = func() ITransformer
	TransformJob struct {
		task.Job
		StreamPath  string           // 对应本地流
		Config      config.Transform // 对应目标流
		Plugin      *Plugin
		Publisher   *Publisher
		Subscriber  *Subscriber
		transformer ITransformer
	}
	DefaultTransformer struct {
		task.Job
		TransformJob TransformJob
	}
)

func (r *DefaultTransformer) GetTransformJob() *TransformJob {
	return &r.TransformJob
}

func (p *TransformJob) GetKey() string {
	return p.StreamPath
}

func (p *TransformJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.transformer, p.StreamPath)
	return
}

func (p *TransformJob) Publish(streamPath string) (err error) {
	p.Publisher, err = p.Plugin.Publish(p.transformer, streamPath)
	return
}

func (p *TransformJob) Init(transformer ITransformer, plugin *Plugin, streamPath string, conf config.Transform) *TransformJob {
	p.Plugin = plugin
	p.Config = conf
	p.StreamPath = streamPath
	p.transformer = transformer
	p.Description = map[string]any{
		"streamPath": streamPath,
		"conf":       conf,
	}
	plugin.Server.Transforms.Add(p, plugin.Logger.With("streamPath", streamPath))
	return p
}

func (p *TransformJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Transforms.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	p.AddTask(p.transformer, p.Logger)
	return
}
