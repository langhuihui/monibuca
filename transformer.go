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
	TransformerFactory = func() ITransformer
	TransformJob       struct {
		task.Job
		StreamPath      string           // 对应本地流
		Config          config.Transform // 对应目标流
		Plugin          *Plugin
		OriginPublisher *Publisher
		Publisher       *Publisher
		Subscriber      *Subscriber
		Transformer     ITransformer
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
	TransformManager struct {
		task.Work
		util.Collection[string, *TransformedMap]
	}
)

func (t *TransformedMap) GetKey() string {
	return t.Target
}

func (r *DefaultTransformer) GetTransformJob() *TransformJob {
	return &r.TransformJob
}

func (p *TransformJob) Subscribe() (err error) {
	subConfig := p.Plugin.config.Subscribe
	subConfig.SubType = SubscribeTypeTransform
	p.Subscriber, err = p.Plugin.SubscribeWithConfig(p.Transformer, p.StreamPath, subConfig)
	if err == nil {
		p.Subscriber.Using(p.Transformer)
	}
	return
}

func (p *TransformJob) Publish(streamPath string) (err error) {
	var conf = p.Plugin.GetCommonConf().Publish
	conf.PubType = PublishTypeTransform
	p.Publisher, err = p.Plugin.PublishWithConfig(context.WithValue(p.Transformer, Owner, p.Transformer), streamPath, conf)
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

func (p *TransformJob) Init(transformer ITransformer, plugin *Plugin, pub *Publisher, conf config.Transform) *TransformJob {
	p.Plugin = plugin
	p.Config = conf
	p.StreamPath = pub.StreamPath
	p.OriginPublisher = pub
	p.Transformer = transformer
	p.SetDescriptions(task.Description{
		"streamPath": pub.StreamPath,
		"conf":       conf,
	})
	transformer.SetRetry(-1, time.Second*2)
	if sender, webhook := plugin.getHookSender(config.HookOnTransformStart); sender != nil {
		transformer.OnStart(func() {
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnTransformStart),
				AlarmType:  config.AlarmTransformRecover,
				StreamPath: pub.StreamPath,
			}
			sender(webhook, alarmInfo)
		})
	}
	if sender, webhook := plugin.getHookSender(config.HookOnTransformEnd); sender != nil {
		transformer.OnDispose(func() {
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnTransformEnd),
				AlarmType:  config.AlarmTransformOffline,
				StreamPath: pub.StreamPath,
				AlarmDesc:  transformer.StopReason().Error(),
			}
			sender(webhook, alarmInfo)
		})
	}
	plugin.Server.Transforms.AddTask(p, plugin.Logger.With("streamPath", pub.StreamPath))
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

func (p *TransformJob) Dispose() {
	transList := &p.Plugin.Server.Transforms
	p.Info("transform -1", "count", transList.Length)
	for _, to := range p.Config.Output {
		transList.RemoveByKey(to.Target)
	}
}
