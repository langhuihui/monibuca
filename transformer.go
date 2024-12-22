package m7s

import (
	"context"
	"slices"
	"time"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
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
		task.Task
		TransformJob TransformJob
	}
	TransformedMap struct {
		StreamPath   string
		Target       string
		TransformJob *TransformJob
	}
	Transforms struct {
		task.Work
		util.Collection[string, *TransformedMap]
		//PublishEvent chan *Publisher
	}
	// TransformsPublishEvent struct {
	// 	task.ChannelTask
	// 	Transforms *Transforms
	// }
)

//func (t *TransformsPublishEvent) GetSignal() any {
//	return t.Transforms.PublishEvent
//}
//
//func (t *TransformsPublishEvent) Tick(pub any) {
//	incomingPublisher := pub.(*Publisher)
//	for job := range t.Transforms.Search(func(m *TransformedMap) bool {
//		return m.StreamPath == incomingPublisher.StreamPath
//	}) {
//		job.TransformJob.TransformPublished(incomingPublisher)
//	}
//}

func (t *TransformedMap) GetKey() string {
	return t.Target
}

func (r *DefaultTransformer) GetTransformJob() *TransformJob {
	return &r.TransformJob
}

func (p *TransformJob) Subscribe() (err error) {
	p.Plugin.config.SubType = SubscribeTypeTransform
	p.Subscriber, err = p.Plugin.Subscribe(p.Transformer, p.StreamPath)
	if err == nil {
		p.Transformer.Depend(p.Subscriber)
	}
	return
}

func (p *TransformJob) Publish(streamPath string) (err error) {
	p.Publisher, err = p.Plugin.Publish(context.WithValue(p.Transformer, Owner, p.Transformer), streamPath)
	p.Publisher.Type = PublishTypeTransform
	if err == nil {
		p.Publisher.OnDispose(func() {
			if p.Publisher.StopReasonIs(pkg.ErrPublishDelayCloseTimeout, task.ErrStopByUser) {
				p.Stop(p.Publisher.StopReason())
			} else {
				p.Transformer.Stop(p.Publisher.StopReason())
			}
		})
	}
	return
}

func (p *TransformJob) Init(transformer ITransformer, plugin *Plugin, streamPath string, conf config.Transform) *TransformJob {
	p.Plugin = plugin
	p.Config = conf
	p.StreamPath = streamPath
	p.Transformer = transformer
	p.SetDescriptions(task.Description{
		"streamPath": streamPath,
		"conf":       conf,
	})
	transformer.SetRetry(-1, time.Second*2)
	plugin.Server.Transforms.AddTask(p, plugin.Logger.With("streamPath", streamPath))
	return p
}

func (p *TransformJob) Start() (err error) {
	s := p.Plugin.Server
	if slices.ContainsFunc(p.Config.Output, func(to config.TransfromOutput) bool {
		return s.Transforms.Has(to.Target)
	}) {
		return pkg.ErrTransformSame
	}
	for _, to := range p.Config.Output {
		if to.Target != "" {
			s.Transforms.Set(&TransformedMap{
				StreamPath:   to.StreamPath,
				Target:       to.Target,
				TransformJob: p,
			})
		}
	}
	p.Info("transform +1", "count", s.Transforms.Length)
	p.AddTask(p.Transformer, p.Logger)
	return
}

//func (p *TransformJob) TransformPublished(pub *Publisher) {
//
//}

func (p *TransformJob) Dispose() {
	transList := &p.Plugin.Server.Transforms
	p.Info("transform -1", "count", transList.Length)
	for _, to := range p.Config.Output {
		transList.RemoveByKey(to.Target)
	}
}
