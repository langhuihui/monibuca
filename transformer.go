package m7s

import (
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/task"
)

type (
	ITransformer interface {
		task.ITask
		GetTransformContext() *TransformContext
	}
	Transformer      = func() ITransformer
	TransformContext struct {
		task.MarcoTask
		FromStreamPath string // 待转换的本地流
		ToStreamPath   string // 转换后的本地流
		Plugin         *Plugin
		Publisher      *Publisher
		Subscriber     *Subscriber
		transformer    ITransformer
	}
	DefaultTransformer struct {
		task.Task
		Ctx TransformContext
	}
)

func (r *DefaultTransformer) GetTransformContext() *TransformContext {
	return &r.Ctx
}

func (r *DefaultTransformer) Start() (err error) {
	return r.Ctx.Subscribe()
}

func (p *TransformContext) GetKey() string {
	return p.ToStreamPath
}

func (p *TransformContext) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.transformer.GetTask().Context, p.FromStreamPath)
	return
}

func (p *TransformContext) Publish() (err error) {
	p.Publisher, err = p.Plugin.Publish(p.transformer.GetTask().Context, p.ToStreamPath)
	return
}

func (p *TransformContext) Init(transformer ITransformer, plugin *Plugin, fromStreamPath string, toStreamPath string) *TransformContext {
	p.Plugin = plugin
	p.FromStreamPath = fromStreamPath
	p.ToStreamPath = toStreamPath
	p.Logger = plugin.Logger.With("fromStreamPath", fromStreamPath, "toStreamPath", toStreamPath)
	if recorderTask := transformer.GetTask(); recorderTask.Logger == nil {
		recorderTask.Logger = p.Logger
	}
	p.transformer = transformer
	plugin.Server.Transforms.Add(p)
	return p
}

func (p *TransformContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Transforms.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	s.Transforms.Add(p)
	s.AddTask(p.transformer)
	return
}

func (p *TransformContext) Dispose() {
	p.Plugin.Server.Transforms.Remove(p)
}
