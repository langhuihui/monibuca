package m7s

import (
	"io"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type (
	Connection struct {
		task.MarcoTask
		Plugin     *Plugin
		StreamPath string // 对应本地流
		RemoteURL  string // 远程服务器地址（用于推拉）
		HTTPClient *http.Client
	}

	IPuller interface {
		task.ITask
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
		task.Task
		Ctx PullContext
		io.ReadCloser
	}
)

func (conn *Connection) Init(plugin *Plugin, streamPath string, href string, proxyConf string) {
	conn.RemoteURL = href
	conn.StreamPath = streamPath
	conn.Plugin = plugin
	conn.HTTPClient = http.DefaultClient
	if proxyConf != "" {
		proxy, err := url.Parse(proxyConf)
		if err != nil {
			return
		}
		transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
		conn.HTTPClient = &http.Client{Transport: transport}
	}
}

func (p *PullContext) GetPullContext() *PullContext {
	return p
}

func (p *PullContext) Init(puller IPuller, plugin *Plugin, streamPath string, url string) *PullContext {
	publishConfig := plugin.config.Publish
	publishConfig.PublishTimeout = 0
	p.Pull = plugin.config.Pull
	p.publishConfig = &publishConfig
	p.Connection.Init(plugin, streamPath, url, plugin.config.Pull.Proxy)
	p.Logger = plugin.Logger.With("pullURL", url, "streamPath", streamPath)
	if pullerTask := puller.GetTask(); pullerTask.Logger == nil {
		pullerTask.Logger = p.Logger
	}
	p.puller = puller
	puller.SetRetry(plugin.config.Pull.RePull, time.Second*5)
	plugin.Server.Pulls.Add(p)
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
		if res, err = p.Ctx.HTTPClient.Get(remoteURL); err == nil {
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
