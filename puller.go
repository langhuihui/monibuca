package m7s

import (
	"crypto/tls"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/format"
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
		Progress      *SubscriptionProgress
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

// Fixed progress steps for HTTP file pull workflow
var httpFilePullSteps = []pkg.StepDef{
	{Name: pkg.StepPublish, Description: "Publishing file stream"},
	{Name: pkg.StepURLParsing, Description: "Determining file source type"},
	{Name: pkg.StepConnection, Description: "Establishing file connection"},
	{Name: pkg.StepParsing, Description: "Parsing file format"},
	{Name: pkg.StepStreaming, Description: "Reading and publishing stream data"},
}

func (conn *Connection) Init(plugin *Plugin, streamPath string, href string, proxyConf string) {
	conn.RemoteURL = href
	conn.StreamPath = streamPath
	conn.Plugin = plugin
	// Create a custom HTTP client that ignores HTTPS certificate validation
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if proxyConf != "" {
		proxy, err := url.Parse(proxyConf)
		if err != nil {
			return
		}
		tr.Proxy = http.ProxyURL(proxy)
	}
	conn.HTTPClient = &http.Client{Transport: tr}
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
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnPullStart),
				StreamPath: streamPath,
				AlarmType:  config.AlarmPullRecover,
			}
			sender(webhook, alarmInfo)
		})
	}

	if sender, webhook := plugin.getHookSender(config.HookOnPullEnd); sender != nil {
		puller.OnDispose(func() {
			p.Fail(puller.StopReason().Error())
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnPullEnd),
				AlarmDesc:  puller.StopReason().Error(),
				StreamPath: streamPath,
				AlarmType:  config.AlarmPullOffline,
			}
			sender(webhook, alarmInfo)
		})
	}

	plugin.Server.Pulls.AddTask(p, plugin.Logger.With("pullURL", conf.URL, "streamPath", streamPath))
	return p
}

func (p *PullJob) GetKey() string {
	return p.StreamPath
}

// Strongly typed helper.
func (p *PullJob) GoToStepConst(name pkg.StepName) {
	if p.Progress == nil {
		return
	}
	// Find step index by name
	stepIndex := -1
	for i, step := range p.Progress.Steps {
		if step.Name == string(name) {
			stepIndex = i
			break
		}
	}
	if stepIndex >= 0 {
		// complete current step if moving forward
		cur := p.Progress.CurrentStep
		if cur != stepIndex {
			cs := &p.Progress.Steps[cur]
			if cs.StartedAt.IsZero() {
				cs.StartedAt = time.Now()
			}
			if cs.CompletedAt.IsZero() {
				cs.CompletedAt = time.Now()
			}
		}
		p.Progress.CurrentStep = stepIndex
		ns := &p.Progress.Steps[stepIndex]
		ns.Error = ""
		if ns.StartedAt.IsZero() {
			ns.StartedAt = time.Now()
		}
	}
}

// Fail marks the current step as failed with an error message
func (p *PullJob) Fail(errorMsg string) {
	if p.Progress == nil {
		return
	}
	idx := p.Progress.CurrentStep
	if idx >= 0 && idx < len(p.Progress.Steps) {
		s := &p.Progress.Steps[idx]
		s.Error = errorMsg
		if s.StartedAt.IsZero() {
			s.StartedAt = time.Now()
		}
		if s.CompletedAt.IsZero() { // mark failed completion time
			s.CompletedAt = time.Now()
		}
	}
}

// SetProgressSteps sets multiple steps from a string array where every two elements represent a step (name, description)
func (p *PullJob) SetProgressStepsDefs(defs []pkg.StepDef) {
	if p.Progress == nil {
		return
	}
	p.Progress.Steps = p.Progress.Steps[:0]
	for _, d := range defs {
		p.Progress.Steps = append(p.Progress.Steps, Step{Name: string(d.Name), Description: d.Description})
	}
}

func (p *PullJob) Publish() (err error) {
	if p.TestMode > 0 {
		return nil
	}
	streamPath := p.StreamPath
	if len(p.Connection.Args) > 0 {
		streamPath += "?" + p.Connection.Args.Encode()
	}
	p.Publisher, err = p.Plugin.PublishWithConfig(p.puller, streamPath, p.PublishConfig)
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
	p.AddTask(p.puller, p.Logger)
	return
}

func (p *HTTPFilePuller) Start() (err error) {
	p.PullJob.SetProgressStepsDefs(httpFilePullSteps)

	if p.PullJob.PublishConfig.Speed == 0 {
		p.PullJob.PublishConfig.Speed = 1 // 对于文件流需要控制速度
	}
	if err = p.PullJob.Publish(); err != nil {
		p.PullJob.Fail(err.Error())
		return
	}
	// move to url_parsing step
	p.PullJob.GoToStepConst(pkg.StepURLParsing)
	if p.ReadCloser != nil {
		return
	}

	p.PullJob.GoToStepConst(pkg.StepConnection)
	remoteURL := p.PullJob.RemoteURL
	p.Info("pull", "remoteurl", remoteURL)
	if strings.HasPrefix(remoteURL, "http") {
		var res *http.Response
		if res, err = p.PullJob.HTTPClient.Get(remoteURL); err == nil {
			if res.StatusCode != http.StatusOK {
				p.PullJob.Fail("HTTP status not OK")
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
	if err != nil {
		p.PullJob.Fail(err.Error())
	}
	p.OnStop(p.ReadCloser.Close)
	return
}

func (p *HTTPFilePuller) GetPullJob() *PullJob {
	return &p.PullJob
}

func (p *HTTPFilePuller) Dispose() {
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

func NewAnnexBPuller(conf config.Pull) IPuller {
	return &AnnexBPuller{}
}

type AnnexBPuller struct {
	HTTPFilePuller
}

func (p *AnnexBPuller) Run() (err error) {
	allocator := util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	defer allocator.Recycle()
	writer := NewPublishVideoWriter[*format.AnnexB](p.PullJob.Publisher, allocator)
	frame := writer.VideoFrame

	p.PullJob.GoToStepConst(pkg.StepParsing) // 解析文件格式

	// 创建 AnnexB 专用读取器
	var annexbReader pkg.AnnexBReader
	var hasFrame bool
	p.PullJob.GoToStepConst(pkg.StepStreaming) // 进入流数据读取阶段
	for !p.IsStopped() {
		// 读取一块数据
		chunkData := allocator.Malloc(8192)
		n, readErr := p.ReadCloser.Read(chunkData)
		if n != 8192 {
			allocator.Free(chunkData[n:])
			chunkData = chunkData[:n]
		}
		if readErr != nil && readErr != io.EOF {
			p.PullJob.Fail(readErr.Error())
			p.Error("读取数据失败", "error", readErr)
			return readErr
		}

		// 将新数据追加到 AnnexB 读取器
		annexbReader.AppendBuffer(chunkData)

		hasFrame, err = frame.Parse(&annexbReader)
		if err != nil {
			p.PullJob.Fail(err.Error())
			return
		}
		if hasFrame {
			frame.SetTS32(uint32(time.Now().UnixMilli()))
			if err = writer.NextVideo(); err != nil {
				p.PullJob.Fail(err.Error())
				return
			}
			frame = writer.VideoFrame
		}
		if readErr == io.EOF {
			return
		}
	}
	return
}
