package m7s

import (
	"fmt"
	"io"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/pkg/util"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type (
	Connection struct {
		task.Job
		Plugin     *Plugin
		StreamPath string // 对应本地流
		Args       url.Values
		RemoteURL  string // 远程服务器地址（用于推拉）
		HTTPClient *http.Client
		Header     http.Header
	}

	IPuller interface {
		task.ITask
		GetPullJob() *PullJob
	}

	Puller = func(config.Pull) IPuller

	PullJob struct {
		Connection
		Publisher     *Publisher
		publishConfig *config.Publish
		puller        IPuller
	}

	HTTPFilePuller struct {
		task.Task
		PullJob PullJob
		io.ReadCloser
	}

	RecordFilePuller struct {
		task.Task
		PullJob       PullJob
		PullStartTime time.Time
		Streams       []RecordStream
		File          *os.File
		offsetTime    time.Duration
	}
)

func (conn *Connection) Init(plugin *Plugin, streamPath string, href string, proxyConf string, header http.Header) {
	conn.RemoteURL = href
	conn.StreamPath = streamPath
	conn.Plugin = plugin
	conn.Header = header
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

func (p *PullJob) GetPullJob() *PullJob {
	return p
}

func (p *PullJob) Init(puller IPuller, plugin *Plugin, streamPath string, conf config.Pull) *PullJob {
	publishConfig := plugin.config.Publish
	publishConfig.PublishTimeout = 0
	p.publishConfig = &publishConfig
	p.Args = conf.Args
	remoteURL := conf.URL
	u, err := url.Parse(remoteURL)
	if err == nil {
		if u.Host == "" {
			// file
			remoteURL = u.Path
		}
		if p.Args == nil {
			p.Args = u.Query()
		} else {
			for k, v := range u.Query() {
				for _, vv := range v {
					p.Args.Add(k, vv)
				}
			}
		}
	}
	p.Connection.Init(plugin, streamPath, remoteURL, conf.Proxy, conf.Header)
	p.puller = puller
	p.Description = map[string]any{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"url":        conf.URL,
		"args":       conf.Args,
		"maxRetry":   conf.MaxRetry,
	}
	puller.SetRetry(conf.MaxRetry, conf.RetryInterval)
	plugin.Server.Pulls.Add(p, plugin.Logger.With("pullURL", conf.URL, "streamPath", streamPath))
	return p
}

func (p *PullJob) GetKey() string {
	return p.StreamPath
}

func (p *PullJob) Publish() (err error) {
	streamPath := p.StreamPath
	if len(p.Args) > 0 {
		streamPath += "?" + p.Args.Encode()
	}
	p.Publisher, err = p.Plugin.PublishWithConfig(p.puller.GetTask().Context, streamPath, *p.publishConfig)
	return
}

func (p *PullJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Pulls.Get(p.GetKey()); ok {
		return pkg.ErrStreamExist
	}
	p.AddTask(p.puller, p.Logger)
	return
}

func (p *HTTPFilePuller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}
	remoteURL := p.PullJob.RemoteURL
	if strings.HasPrefix(remoteURL, "http") {
		var res *http.Response
		if res, err = p.PullJob.HTTPClient.Get(remoteURL); err == nil {
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
		//p.PullJob.Publisher.Publish.Speed = 1
	}
	return
}

func (p *HTTPFilePuller) GetPullJob() *PullJob {
	return &p.PullJob
}

func (p *HTTPFilePuller) Dispose() {
	p.ReadCloser.Close()
}

func (p *RecordFilePuller) GetPullJob() *PullJob {
	return &p.PullJob
}

func (p *RecordFilePuller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}
	if p.PullStartTime, err = util.TimeQueryParse(p.PullJob.Args.Get("start")); err != nil {
		return
	}

	tx := p.PullJob.Plugin.DB.Find(&p.Streams, "end_time>? AND file_path=?", p.PullStartTime, p.PullJob.RemoteURL)
	if tx.Error != nil {
		return tx.Error
	}
	if len(p.Streams) == 0 {
		return fmt.Errorf("stream not found")
	}
	p.Info("vod", "streams", p.Streams)
	return
}
