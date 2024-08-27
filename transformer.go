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
		StreamPath  string // 对应本地流
		Target      string // 对应目标流
		Plugin      *Plugin
		Publisher   *Publisher
		Subscriber  *Subscriber
		transformer ITransformer
	}
	DefaultTransformer struct {
		task.Task
		TransformJob TransformJob
	}
)

func (r *DefaultTransformer) GetTransformJob() *TransformJob {
	return &r.TransformJob
}

func (r *DefaultTransformer) Start() (err error) {
	err = r.TransformJob.Subscribe()
	if err == nil {
		err = r.TransformJob.Publish()
	}
	return
}

func (p *TransformJob) GetKey() string {
	return p.Target
}

func (p *TransformJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.transformer, p.StreamPath)
	return
}

func (p *TransformJob) Publish() (err error) {
	p.Publisher, err = p.Plugin.Publish(p.transformer, p.Target)
	return
}

func (p *TransformJob) Init(transformer ITransformer, plugin *Plugin, streamPath string, conf config.Transform) *TransformJob {
	p.Plugin = plugin
	p.Target = conf.Target
	p.StreamPath = streamPath
	p.transformer = transformer
	p.Description = map[string]any{
		"streamPath": streamPath,
		"target":     conf.Target,
		"conf":       conf.Conf,
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
