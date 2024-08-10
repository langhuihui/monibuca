package m7s

import (
	"context"
	"time"

	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
)

type Connection struct {
	pkg.MarcoTask
	Plugin         *Plugin
	StreamPath     string // 对应本地流
	RemoteURL      string // 远程服务器地址（用于推拉）
	ReConnectCount int    //重连次数
	ConnectProxy   string // 连接代理
	MetaData       any
}

func (client *Connection) reconnect(count int) (ok bool) {
	ok = count == -1 || client.ReConnectCount <= count
	client.ReConnectCount++
	return
}

type Puller = func(*PullContext) error

func createPullContext(p *Plugin, streamPath string, url string, options ...any) (pullCtx *PullContext) {
	pullCtx = &PullContext{Pull: p.config.Pull}
	pullCtx.Plugin = p
	pullCtx.ConnectProxy = p.config.Pull.Proxy
	pullCtx.RemoteURL = url
	publishConfig := p.config.Publish
	publishConfig.PublishTimeout = 0
	pullCtx.StreamPath = streamPath
	pullCtx.PublishOptions = []any{publishConfig}
	var ctx = p.Context
	for _, option := range options {
		switch v := option.(type) {
		case context.Context:
			ctx = v
		default:
			pullCtx.PublishOptions = append(pullCtx.PublishOptions, option)
		}
	}
	p.InitKeepAlive(ctx, p.Logger.With("pullURL", url, "streamPath", streamPath), pullCtx)
	pullCtx.PublishOptions = append(pullCtx.PublishOptions, pullCtx.Context)
	p.Server.pullTask.AddTask(pullCtx)
	return
}

type PullContext struct {
	Connection
	Publisher      *Publisher
	PublishOptions []any
	config.Pull
}

func (p *PullContext) GetKey() string {
	return p.StreamPath
}

func (p *PullContext) Do(puller Puller) {
	p.AddCall(func(tmpTask *pkg.Task) (err error) {
		publishOptions := append([]any{tmpTask.Context}, p.PublishOptions...)
		if p.Publisher, err = p.Plugin.Publish(p.StreamPath, publishOptions...); err != nil {
			p.Error("pull publish failed", "error", err)
			return
		}
		err = puller(p)
		if p.reconnect(p.RePull) {
			if time.Since(tmpTask.StartTime) < 5*time.Second {
				time.Sleep(5 * time.Second)
			}
			p.Warn("retry", "count", p.ReConnectCount, "total", p.RePull)
			p.Do(puller)
		} else {
			p.Stop(pkg.ErrRetryRunOut)
		}
		return
	}, nil)
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
