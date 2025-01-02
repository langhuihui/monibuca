package m7s

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
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
		PublishConfig config.Publish
		puller        IPuller
		conf          *config.Pull
	}

	HTTPFilePuller struct {
		task.Task
		PullJob PullJob
		io.ReadCloser
	}

	RecordFilePuller struct {
		task.Task
		PullJob                    PullJob
		PullStartTime, PullEndTime time.Time
		Streams                    []RecordStream
		File                       *os.File
		MaxTS                      int64
	}

	wsReadCloser struct {
		ws *websocket.Conn
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

func (p *PullJob) Init(puller IPuller, plugin *Plugin, streamPath string, conf config.Pull, pubConf *config.Publish) *PullJob {
	if pubConf == nil {
		p.PublishConfig = plugin.GetCommonConf().Publish
	} else {
		p.PublishConfig = *pubConf
	}
	p.PublishConfig.PubType = PublishTypePull
	p.Args = url.Values(conf.Args.DeepClone())
	p.conf = &conf
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
	p.Connection.Init(plugin, streamPath, remoteURL, conf.Proxy, http.Header(conf.Header))
	p.puller = puller
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"url":        conf.URL,
		"args":       conf.Args,
		"maxRetry":   conf.MaxRetry,
	})
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
	p.Publisher, err = p.Plugin.PublishWithConfig(p.puller.GetTask().Context, streamPath, p.PublishConfig)
	if err == nil && p.conf.MaxRetry != 0 {
		p.Publisher.OnDispose(func() {
			if p.Publisher.StopReasonIs(pkg.ErrPublishDelayCloseTimeout, task.ErrStopByUser) {
				p.Stop(p.Publisher.StopReason())
			} else {
				p.puller.Stop(p.Publisher.StopReason())
			}
		})
	}
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
	} else if strings.HasPrefix(remoteURL, "ws") {
		var ws *websocket.Conn
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}
		if ws, _, err = dialer.Dial(remoteURL, nil); err == nil {
			p.ReadCloser = &wsReadCloser{ws: ws}
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
	p.SetRetry(0, 0)
	if p.PullJob.Plugin.DB == nil {
		return pkg.ErrNoDB
	}
	p.PullJob.PublishConfig.PubType = PublishTypeVod
	if err = p.PullJob.Publish(); err != nil {
		return
	}
	if p.PullStartTime, p.PullEndTime, err = util.TimeRangeQueryParse(p.PullJob.Args); err != nil {
		return
	}
	queryRecord := RecordStream{
		Mode: RecordModeAuto,
	}
	tx := p.PullJob.Plugin.DB.Where(&queryRecord).Find(&p.Streams, "end_time>=? AND start_time<=? AND stream_path=?", p.PullStartTime, p.PullEndTime, p.PullJob.RemoteURL)
	if tx.Error != nil {
		return tx.Error
	}
	p.MaxTS = p.PullEndTime.Sub(p.PullStartTime).Milliseconds()

	if len(p.Streams) == 0 {
		return pkg.ErrNotFound
	}
	p.Info("vod", "streams", p.Streams)
	return
}

func (p *RecordFilePuller) Dispose() {
	if p.File != nil {
		p.File.Close()
	}
}

func (w *wsReadCloser) Read(p []byte) (n int, err error) {
	_, message, err := w.ws.ReadMessage()
	if err != nil {
		return 0, err
	}
	return copy(p, message), nil
}

func (w *wsReadCloser) Close() error {
	return w.ws.Close()
}
