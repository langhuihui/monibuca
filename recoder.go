package m7s

import (
	"gorm.io/gorm"
	"time"

	"m7s.live/v5/pkg/config"

	"m7s.live/v5/pkg/task"

	"m7s.live/v5/pkg"
)

type (
	IRecorder interface {
		task.ITask
		GetRecordJob() *RecordJob
	}
	Recorder  = func() IRecorder
	RecordJob struct {
		task.Job
		StreamPath string // 对应本地流
		Plugin     *Plugin
		Subscriber *Subscriber
		Fragment   time.Duration
		Append     bool
		FilePath   string
		recorder   IRecorder
	}
	DefaultRecorder struct {
		task.Task
		RecordJob RecordJob
	}
	RecordStream struct {
		ID                     uint `gorm:"primarykey"`
		StartTime, EndTime     time.Time
		EventId                string `json:"eventId" desc:"事件编号" gorm:"type:varchar(255);comment:事件编号"`
		RecordMode             string `json:"recordMode" desc:"事件类型,0=连续录像模式，1=事件录像模式" gorm:"type:varchar(255);comment:事件类型,0=连续录像模式，1=事件录像模式;default:'0'"`
		EventName              string `json:"eventName" desc:"事件名称" gorm:"type:varchar(255);comment:事件名称"`
		BeforeDuration         string `json:"beforeDuration" desc:"事件前缓存时长" gorm:"type:varchar(255);comment:事件前缓存时长;default:'30s'"`
		AfterDuration          string `json:"afterDuration" desc:"事件后缓存时长" gorm:"type:varchar(255);comment:事件后缓存时长;default:'30s'"`
		Filename               string `json:"fileName" desc:"文件名" gorm:"type:varchar(255);comment:文件名"`
		EventDesc              string `json:"eventDesc" desc:"事件描述" gorm:"type:varchar(255);comment:事件描述"`
		Type                   string `json:"type" desc:"录像文件类型" gorm:"type:varchar(255);comment:录像文件类型,flv,mp4,raw,fmp4,hls"`
		EventLevel             string `json:"eventLevel" desc:"事件级别" gorm:"type:varchar(255);comment:事件级别,0表示重要事件，无法删除且表示无需自动删除,1表示非重要事件,达到自动删除时间后，自动删除;default:'1'"`
		FilePath               string
		StreamPath             string
		AudioCodec, VideoCodec string
		DeletedAt              gorm.DeletedAt `gorm:"index" yaml:"-"`
	}
)

func (r *DefaultRecorder) GetRecordJob() *RecordJob {
	return &r.RecordJob
}

func (r *DefaultRecorder) Start() (err error) {
	return r.RecordJob.Subscribe()
}

func (p *RecordJob) GetKey() string {
	return p.FilePath
}

func (p *RecordJob) Subscribe() (err error) {
	p.Subscriber, err = p.Plugin.Subscribe(p.recorder.GetTask().Context, p.StreamPath)
	return
}

func (p *RecordJob) Init(recorder IRecorder, plugin *Plugin, streamPath string, conf config.Record) *RecordJob {
	p.Plugin = plugin
	p.Fragment = conf.Fragment
	p.Append = conf.Append
	p.FilePath = conf.FilePath
	p.StreamPath = streamPath
	p.recorder = recorder
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"filePath":   conf.FilePath,
		"append":     conf.Append,
		"fragment":   conf.Fragment,
	})
	plugin.Server.Records.Add(p, plugin.Logger.With("filePath", conf.FilePath, "streamPath", streamPath))
	return p
}

func (p *RecordJob) Start() (err error) {
	s := p.Plugin.Server
	if _, ok := s.Records.Get(p.GetKey()); ok {
		return pkg.ErrRecordSamePath
	}
	// dir := p.FilePath
	// if p.Fragment == 0 || p.Append {
	// 	dir = filepath.Dir(p.FilePath)
	// }
	// p.SetDescription("filePath", p.FilePath)
	// if err = os.MkdirAll(dir, 0755); err != nil {
	// 	return
	// }
	p.AddTask(p.recorder, p.Logger)
	return
}
