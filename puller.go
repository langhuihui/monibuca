package m7s

import (
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
)

type Connection struct {
	util.MarcoTask
	Plugin       *Plugin
	StreamPath   string // 对应本地流
	RemoteURL    string // 远程服务器地址（用于推拉）
	ConnectProxy string // 连接代理
}

type Puller = func(*PullContext) error

func createPullContext(p *Plugin, streamPath string, url string) (pullCtx *PullContext) {
	publishConfig := p.config.Publish
	publishConfig.PublishTimeout = 0
	pullCtx = &PullContext{
		Pull:          p.config.Pull,
		publishConfig: &publishConfig,
	}
	pullCtx.Name = "pull"
	pullCtx.Plugin = p
	pullCtx.ConnectProxy = p.config.Pull.Proxy
	pullCtx.RemoteURL = url
	pullCtx.StreamPath = streamPath
	pullCtx.Logger = p.Logger.With("pullURL", url, "streamPath", streamPath)
	return
}

type PullContext struct {
	Connection
	Publisher     *Publisher
	publishConfig *config.Publish
	config.Pull
}

func (p *PullContext) GetKey() string {
	return p.StreamPath
}

type PullSubTask struct {
	util.RetryTask
	ctx *PullContext
	Puller
}

func (p *PullSubTask) Start() (err error) {
	p.MaxRetry = p.ctx.RePull
	if p.ctx.Publisher, err = p.ctx.Plugin.PublishWithConfig(p.Context, p.ctx.StreamPath, *p.ctx.publishConfig); err != nil {
		p.Error("pull publish failed", "error", err)
		return
	}
	p.ctx.Publisher.OnDispose(func() {
		p.Stop(p.ctx.Publisher.StopReason())
	})
	return p.Puller(p.ctx)
}

func (p *PullContext) Do(puller Puller) {
	p.AddTask(&PullSubTask{ctx: p, Puller: puller})
}

func (p *PullContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pulls.Get(p.GetKey()); ok {
		return pkg.ErrStreamExist
	}
	s.Pulls.Add(p)
	if p.Plugin.Meta.Puller != nil {
		p.Do(p.Plugin.Meta.Puller)
	}
	return
}

func (p *PullContext) Dispose() {
	p.Plugin.Server.Pulls.Remove(p)
}
