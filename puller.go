package m7s

import (
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	}

	IPuller interface {
		task.ITask
		GetPullJob() *PullJob
	}

	PullerFactory = func(config.Pull) IPuller

	PullJob struct {
		Connection
		*config.Pull
		Publisher     *Publisher
		PublishConfig config.Publish
		puller        IPuller
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
		seekChan                   chan time.Time
		Type                       string
		Loop                       int
	}

	wsReadCloser struct {
		ws *websocket.Conn
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
	p.Connection.Args = url.Values(conf.Args.DeepClone())
	p.Pull = &conf
	remoteURL := conf.URL
	u, err := url.Parse(remoteURL)
	if err == nil {
		if u.Host == "" {
			// file
			remoteURL = u.Path
		}
		if p.Connection.Args == nil {
			p.Connection.Args = u.Query()
		} else {
			for k, v := range u.Query() {
				for _, vv := range v {
					p.Connection.Args.Add(k, vv)
				}
			}
		}
	}
	p.Connection.Init(plugin, streamPath, remoteURL, conf.Proxy)
	p.puller = puller
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"url":        conf.URL,
		"args":       conf.Args,
		"maxRetry":   conf.MaxRetry,
	})
	puller.SetRetry(conf.MaxRetry, conf.RetryInterval)

	if sender, webhook := plugin.getHookSender(config.HookOnPullStart); sender != nil {
		puller.OnStart(func() {
			webhookData := map[string]interface{}{
				"alarmDesc":  config.HookOnPullStart,
				"event":      config.HookOnPullStart,
				"streamPath": streamPath,
				"url":        conf.URL,
				"args":       conf.Args,
				"pluginName": plugin.Meta.Name,
				"timestamp":  time.Now().Unix(),
				"alarmType":  config.AlarmPullRecover,
			}
			sender(webhook, webhookData)
		})
	}

	if sender, webhook := plugin.getHookSender(config.HookOnPullEnd); sender != nil {
		puller.OnDispose(func() {
			webhookData := map[string]interface{}{
				"alarmDesc":  puller.StopReason().Error(),
				"event":      config.HookOnPullEnd,
				"streamPath": streamPath,
				"reason":     puller.StopReason().Error(),
				"timestamp":  time.Now().Unix(),
				"alarmType":  config.AlarmPullOffline,
			}
			sender(webhook, webhookData)
		})
	}

	plugin.Server.Pulls.Add(p, plugin.Logger.With("pullURL", conf.URL, "streamPath", streamPath))
	return p
}

func (p *PullJob) GetKey() string {
	return p.StreamPath
}

func (p *PullJob) Publish() (err error) {
	if p.TestMode > 0 {
		return nil
	}
	streamPath := p.StreamPath
	if len(p.Connection.Args) > 0 {
		streamPath += "?" + p.Connection.Args.Encode()
	}
	p.Publisher, err = p.Plugin.PublishWithConfig(p.puller.GetTask().Context, streamPath, p.PublishConfig)
	if err == nil {
		p.Publisher.OnDispose(func() {
			if p.Publisher.StopReasonIs(pkg.ErrPublishDelayCloseTimeout, task.ErrStopByUser) || p.MaxRetry == 0 {
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
	if p.ReadCloser != nil {
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
	p.ReadCloser = nil
}

func (p *RecordFilePuller) GetPullJob() *PullJob {
	return &p.PullJob
}

func (p *RecordFilePuller) queryRecordStreams(startTime, endTime time.Time) (err error) {
	if p.PullJob.Plugin.DB == nil {
		return pkg.ErrNoDB
	}
	queryRecord := RecordStream{
		Type: p.Type,
	}
	tx := p.PullJob.Plugin.DB.Where(&queryRecord).Find(&p.Streams, "end_time>=? AND start_time<=? AND stream_path=?", startTime, endTime, p.PullJob.RemoteURL)
	if tx.Error != nil {
		return tx.Error
	}
	if len(p.Streams) == 0 {
		return pkg.ErrNotFound
	}
	for _, stream := range p.Streams {
		p.Debug("queryRecordStreams", "filePath", stream.FilePath)
	}
	p.MaxTS = endTime.Sub(startTime).Milliseconds()
	return nil
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
	if p.PullStartTime, p.PullEndTime, err = util.TimeRangeQueryParse(p.PullJob.Connection.Args); err != nil {
		return
	}
	p.seekChan = make(chan time.Time, 1)
	loop := p.PullJob.Connection.Args.Get(util.LoopKey)
	p.Loop, err = strconv.Atoi(loop)
	if err != nil || p.Loop < 0 {
		p.Loop = math.MaxInt32
	}
	publisher := p.PullJob.Publisher
	if publisher != nil {
		publisher.OnSeek = func(seekTime time.Time) {
			// p.PullStartTime = seekTime
			// p.SetRetry(1, 0)
			// if util.UnixTimeReg.MatchString(p.PullJob.Args.Get(util.EndKey)) {
			// 	p.PullJob.Args.Set(util.StartKey, strconv.FormatInt(seekTime.Unix(), 10))
			// } else {
			// 	p.PullJob.Args.Set(util.StartKey, seekTime.Local().Format(util.LocalTimeFormat))
			// }
			select {
			case p.seekChan <- seekTime:
			default:
			}
		}
	}
	return p.queryRecordStreams(p.PullStartTime, p.PullEndTime)
}

func (p *RecordFilePuller) GetSeekChan() chan time.Time {
	return p.seekChan
}

func (p *RecordFilePuller) Dispose() {
	if p.File != nil {
		p.File.Close()
	}
	close(p.seekChan)
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

func (p *RecordFilePuller) CheckSeek() (needSeek bool, err error) {
	select {
	case p.PullStartTime = <-p.seekChan:
		if err = p.queryRecordStreams(p.PullStartTime, p.PullEndTime); err != nil {
			return
		}
		if p.File != nil {
			p.File.Close()
			p.File = nil
		}
		needSeek = true
	default:
	}
	return
}
