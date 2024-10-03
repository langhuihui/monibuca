package m7s

import (
	"context"

	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/pkg/util"
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
		Transformer ITransformer
	}
	DefaultTransformer struct {
		task.Job
		TransformJob TransformJob
	}
	TransformedMap struct {
		StreamPath   string
		TransformJob *TransformJob
	}
	Transforms struct {
		Transformed util.Collection[string, *TransformedMap]
		task.Manager[string, *TransformJob]
		PublishEvent chan *Publisher
	}
	TransformsPublishEvent struct {
		task.ChannelTask
		Transforms *Transforms
	}
)

func (t *TransformsPublishEvent) GetSignal() any {
	return t.Transforms.PublishEvent
}

func (t *TransformsPublishEvent) Tick(pub any) {
	if m, ok := t.Transforms.Transformed.Get(pub.(*Publisher).StreamPath); ok {
		m.TransformJob.TransformPublished(pub.(*Publisher))
	}
}

func (t *TransformedMap) GetKey() string {
	return t.StreamPath
}

func (r *DefaultTransformer) GetTransformJob() *TransformJob {
	return &r.TransformJob
}

func (p *TransformJob) GetKey() string {
	return p.StreamPath
}

func (p *TransformJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.Transformer, p.StreamPath)
	return
}

func (p *TransformJob) Publish(streamPath string) (err error) {
	p.Publisher, err = p.Plugin.Publish(context.WithValue(p.Transformer, Owner, p.Transformer), streamPath)
	return
}

func (p *TransformJob) Init(transformer ITransformer, plugin *Plugin, streamPath string, conf config.Transform) *TransformJob {
	p.Plugin = plugin
	p.Config = conf
	p.StreamPath = streamPath
	p.Transformer = transformer
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
		return pkg.ErrTransformSame
	}

	if _, ok := s.Transforms.Transformed.Get(p.GetKey()); ok {
		return pkg.ErrStreamExist
	}

	for _, to := range p.Config.Output {
		if to.StreamPath != "" {
			s.Transforms.Transformed.Set(&TransformedMap{
				StreamPath:   to.StreamPath,
				TransformJob: p,
			})
		}
	}
	p.AddTask(p.Transformer, p.Logger)
	return
}

func (p *TransformJob) TransformPublished(pub *Publisher) {
	p.Publisher = pub
	pub.OnDispose(func() {
		p.Stop(pub.StopReason())
	})
}

func (p *TransformJob) Dispose() {
	for _, to := range p.Config.Output {
		if to.StreamPath != "" {
			p.Plugin.Server.Transforms.Transformed.RemoveByKey(to.StreamPath)
		}
	}
}
