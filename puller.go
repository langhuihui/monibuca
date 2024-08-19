package m7s

import (
	"io"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type (
	Connection struct {
		util.MarcoTask
		Plugin       *Plugin
		StreamPath   string // 对应本地流
		RemoteURL    string // 远程服务器地址（用于推拉）
		ConnectProxy string // 连接代理
	}

	IPuller interface {
		util.ITask
		GetPullContext() *PullContext
	}

	Puller = func() IPuller

	PullContext struct {
		Connection
		Publisher     *Publisher
		publishConfig *config.Publish
		config.Pull
		puller IPuller
	}

	HttpFilePuller struct {
		util.Task
		Ctx PullContext
		io.ReadCloser
	}
)

func (p *PullContext) GetPullContext() *PullContext {
	return p
}

func (p *PullContext) Init(puller IPuller, plugin *Plugin, streamPath string, url string) *PullContext {
	publishConfig := plugin.config.Publish
	publishConfig.PublishTimeout = 0
	p.Pull = plugin.config.Pull
	p.publishConfig = &publishConfig
	p.Plugin = plugin
	p.ConnectProxy = plugin.config.Pull.Proxy
	p.RemoteURL = url
	p.StreamPath = streamPath
	p.Logger = p.Logger.With("pullURL", url, "streamPath", streamPath)
	p.puller = puller
	puller.SetRetry(plugin.config.Pull.RePull, time.Second*5)
	return p
}

func (p *PullContext) GetKey() string {
	return p.StreamPath
}

func (p *PullContext) Publish() (err error) {
	p.Publisher, err = p.Plugin.PublishWithConfig(p.puller.GetTask().Context, p.StreamPath, *p.publishConfig)
	return
}

func (p *PullContext) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pulls.Get(p.GetKey()); ok {
		return pkg.ErrStreamExist
	}
	s.Pulls.Add(p)
	s.AddTask(p.puller)
	return
}

func (p *PullContext) Dispose() {
	p.Plugin.Server.Pulls.Remove(p)
}

func (p *HttpFilePuller) Start() (err error) {
	if err = p.Ctx.Publish(); err != nil {
		return
	}
	remoteURL := p.Ctx.RemoteURL
	if strings.HasPrefix(remoteURL, "http") {
		var res *http.Response
		client := http.DefaultClient
		if proxyConf := p.Ctx.ConnectProxy; proxyConf != "" {
			proxy, err := url.Parse(proxyConf)
			if err != nil {
				return err
			}
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			client = &http.Client{Transport: transport}
		}
		if res, err = client.Get(remoteURL); err == nil {
			if res.StatusCode != http.StatusOK {
				return io.EOF
			}
			p.ReadCloser = res.Body
		}
	} else {
		var res *os.File
		if res, err = os.Open(remoteURL); err == nil {
			p.ReadCloser = res
		}
	}
	return
}

func (p *HttpFilePuller) GetPullContext() *PullContext {
	return &p.Ctx
}

func (p *HttpFilePuller) Dispose() {
	p.ReadCloser.Close()
}
