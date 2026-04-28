package m7s

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/storage"
)

type (
	IRecorder interface {
		task.ITask
		GetRecordJob() *RecordJob
	}
	RecorderFactory = func(config.Record) IRecorder
	// RecordEvent 包含录像事件的公共字段

	EventRecordStream struct {
		*config.RecordEvent
		RecordStream
	}
	RecordJob struct {
		task.Job
		Event      *config.RecordEvent
		StreamPath string // 对应本地流
		Plugin     *Plugin
		Subscriber *Subscriber
		SubConf    *config.Subscribe
		RecConf    *config.Record
		recorder   IRecorder
		storage    storage.Storage // 存储实例
	}
	DefaultRecorder struct {
		task.Task
		RecordJob RecordJob
		Event     EventRecordStream
	}
	RecordStream struct {
		ID           uint      `gorm:"primarykey"`
		StartTime    time.Time `gorm:"default:NULL"`
		EndTime      time.Time `gorm:"default:NULL"`
		Duration     uint32    `gorm:"comment:录像时长;default:0"`
		Filename     string    `json:"fileName" desc:"文件名" gorm:"type:varchar(255);comment:文件名"`
		Type         string    `json:"type" desc:"录像文件类型" gorm:"type:varchar(255);comment:录像文件类型,flv,mp4,raw,fmp4,hls"`
		FilePath     string
		StreamPath   string
		AudioCodec   string
		VideoCodec   string
		CreatedAt    time.Time
		DeletedAt    gorm.DeletedAt    `gorm:"index" yaml:"-"`
		RecordLevel  config.EventLevel `json:"eventLevel" desc:"事件级别" gorm:"type:varchar(255);comment:事件级别,high表示重要事件，无法删除且表示无需自动删除,low表示非重要事件,达到自动删除时间后，自动删除;default:'low'"`
		StorageLevel int               `json:"storageLevel" desc:"存储级别" gorm:"comment:存储级别,1=主存储,2=次级存储;default:1"`
		StorageType  string            `json:"storageType" desc:"存储类型" gorm:"type:varchar(20);comment:存储类型(local/s3/oss/cos);default:'local'"`
	}
)

func (r *DefaultRecorder) GetRecordJob() *RecordJob {
	return &r.RecordJob
}

func (r *DefaultRecorder) Start() (err error) {
	return r.RecordJob.Subscribe()
}

func (r *DefaultRecorder) CreateStream(start time.Time, customFileName func(*RecordJob) string) (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber

	// 生成文件路径
	filePath := customFileName(recordJob)

	var storageType string
	recordJob.storage = recordJob.Plugin.Server.Storage
	if recordJob.storage != nil {
		storageType = recordJob.storage.GetKey()
	}

	if recordJob.storage == nil {
		return fmt.Errorf("storage config is required")
	}

	r.Event.RecordStream = RecordStream{
		StartTime:    start,
		StreamPath:   sub.StreamPath,
		FilePath:     filePath,
		Type:         recordJob.RecConf.Type,
		StorageLevel: 1, // 默认为主存储
		StorageType:  storageType,
	}

	if sub.Publisher.HasAudioTrack() {
		r.Event.AudioCodec = sub.Publisher.AudioTrack.ICodecCtx.String()
	}
	if sub.Publisher.HasVideoTrack() {
		r.Event.VideoCodec = sub.Publisher.VideoTrack.ICodecCtx.String()
	}
	if recordJob.Plugin.DB != nil && recordJob.RecConf.Mode != config.RecordModeTest {
		if recordJob.Event != nil {
			r.Event.RecordEvent = recordJob.Event
			r.Event.RecordLevel = recordJob.Event.EventLevel
			recordJob.Plugin.DB.Save(&r.Event.RecordStream)
			recordJob.Plugin.DB.Save(&r.Event)
		} else {
			recordJob.Plugin.DB.Save(&r.Event.RecordStream)
		}
	}
	recordJob.SetDescription("streamPath", recordJob.StreamPath)
	recordJob.SetDescription("fileName", filePath)
	recordJob.SetDescription("startTime", start.Format("2006-01-02 15:04:05"))
	return
}

// createStorage 创建存储实例，返回 storage 和存储类型
func (r *DefaultRecorder) createStorage(storageConfig map[string]any) (storage.Storage, string) {
	for t, conf := range storageConfig {
		r.Info("trying to create storage", "type", t, "config", conf)
		storage, err := storage.CreateStorage(t, conf)
		if err == nil {
			r.Info("storage created successfully", "type", t)
			return storage, t
		}
		r.Error("create storage failed", "type", t, "err", err)
	}
	r.Warn("falling back to local storage", "path", r.RecordJob.RecConf.FilePath)
	localStorage, err := storage.CreateStorage("local", r.RecordJob.RecConf.FilePath)
	if err == nil {
		return localStorage, "local"
	} else {
		r.Error("create local storage failed", "err", err)
	}
	return nil, ""
}

func (r *DefaultRecorder) WriteTail(end time.Time, tailJob task.IJob) {
	r.Event.EndTime = end
	if r.RecordJob.Plugin.DB != nil && r.RecordJob.RecConf.Mode != config.RecordModeTest {
		// 将事件和录像记录关联
		if r.RecordJob.Event != nil {
			r.RecordJob.Plugin.DB.Save(&r.Event)
			r.RecordJob.Plugin.DB.Save(&r.Event.RecordStream)
		} else {
			r.RecordJob.Plugin.DB.Save(&r.Event.RecordStream)
		}
		if tailJob == nil {
			return
		}
		tailJob.AddTask(NewEventRecordCheck(r.Event.Type, r.Event.StreamPath, r.RecordJob.Plugin.DB))
	}
}

// WriteTailDeferred 设置结束时间，并返回一个延迟执行的 DB 写入函数。
// 与 WriteTail 不同，它不立即写库，而是将写库操作包装成闭包返回。
// 调用方应在 MP4 文件完整写入（moov 移到头部）后再调用该闭包，
// 以保证数据库记录在文件可播放之后才更新 EndTime。
func (r *DefaultRecorder) WriteTailDeferred(end time.Time) func(tailJob task.IJob) {
	r.Event.EndTime = end
	if r.RecordJob.Plugin.DB == nil || r.RecordJob.RecConf.Mode == config.RecordModeTest {
		return nil
	}
	db := r.RecordJob.Plugin.DB
	streamType := r.Event.Type
	streamPath := r.Event.StreamPath
	filePath := r.Event.FilePath
	if r.RecordJob.Event != nil {
		eventSnap := r.Event // 值拷贝：捕获正确的 EndTime 和 RecordStream.ID，RecordEvent 指针稳定
		return func(tailJob task.IJob) {
			dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r.Info("db save RecordEvent (deferred) begin", "filePath", filePath)
			if result := db.WithContext(dbCtx).Save(&eventSnap); result.Error != nil {
				r.Warn("db save RecordEvent (deferred) failed", "filePath", filePath, "err", result.Error)
			} else {
				r.Info("db save RecordEvent (deferred) ok", "filePath", filePath)
			}
			r.Info("db save RecordStream (deferred) begin", "filePath", filePath)
			if result := db.WithContext(dbCtx).Save(&eventSnap.RecordStream); result.Error != nil {
				r.Warn("db save RecordStream (deferred) failed", "filePath", filePath, "err", result.Error)
			} else {
				r.Info("db save RecordStream (deferred) ok", "filePath", filePath)
			}
			if tailJob != nil {
				tailJob.AddTask(NewEventRecordCheck(streamType, streamPath, db))
			}
		}
	}
	streamSnap := r.Event.RecordStream // 值拷贝
	return func(tailJob task.IJob) {
		dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r.Info("db save RecordStream (deferred) begin", "filePath", filePath)
		if result := db.WithContext(dbCtx).Save(&streamSnap); result.Error != nil {
			r.Warn("db save RecordStream (deferred) failed", "filePath", filePath, "err", result.Error)
		} else {
			r.Info("db save RecordStream (deferred) ok", "filePath", filePath)
		}
		if tailJob != nil {
			tailJob.AddTask(NewEventRecordCheck(streamType, streamPath, db))
		}
	}
}

func (p *RecordJob) GetKey() string {
	return p.RecConf.FilePath
}

// GetStorage 获取存储实例
func (p *RecordJob) GetStorage() storage.Storage {
	return p.storage
}

func (p *RecordJob) Subscribe() (err error) {

	p.Subscriber, err = p.Plugin.SubscribeWithConfig(p.recorder.GetTask().Context, p.StreamPath, *p.SubConf)
	return
}

func (p *RecordJob) Init(recorder IRecorder, plugin *Plugin, streamPath string, conf config.Record, subConf *config.Subscribe) *RecordJob {
	p.Plugin = plugin
	p.RecConf = &conf
	p.Event = conf.Event
	p.StreamPath = streamPath
	if subConf == nil {
		conf := p.Plugin.config.Subscribe
		subConf = &conf
	}
	subConf.SubType = SubscribeTypeVod
	p.SubConf = subConf
	p.recorder = recorder
	p.SetDescriptions(task.Description{
		"plugin":     plugin.Meta.Name,
		"streamPath": streamPath,
		"filePath":   conf.FilePath,
		"append":     conf.Append,
		"fragment":   conf.Fragment,
	})
	recorder.SetRetry(-1, time.Second)
	if sender, webhook := plugin.getHookSender(config.HookOnRecordStart); sender != nil {
		recorder.OnStart(func() {
			alarmInfo := AlarmInfo{
				AlarmName:  string(config.HookOnRecordStart),
				AlarmType:  config.AlarmStorageExceptionRecover,
				StreamPath: streamPath,
				FilePath:   conf.FilePath,
			}
			sender(webhook, alarmInfo)
		})
	}

	if sender, webhook := plugin.getHookSender(config.HookOnRecordEnd); sender != nil {
		recorder.OnDispose(func() {
			alarmInfo := AlarmInfo{
				AlarmType:  config.AlarmStorageException,
				AlarmDesc:  recorder.StopReason().Error(),
				AlarmName:  string(config.HookOnRecordEnd),
				StreamPath: streamPath,
				FilePath:   conf.FilePath,
			}
			sender(webhook, alarmInfo)
		})
	}

	plugin.Server.Records.AddTask(p, plugin.Logger.With("filePath", conf.FilePath, "streamPath", streamPath))
	return p
}

func (p *RecordJob) Start() (err error) {
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

func NewEventRecordCheck(t string, streamPath string, db *gorm.DB) *eventRecordCheck {
	return &eventRecordCheck{
		DB:         db,
		streamPath: streamPath,
		Type:       t,
	}
}

type eventRecordCheck struct {
	task.Task
	DB         *gorm.DB
	streamPath string
	Type       string
}

func (t *eventRecordCheck) Run() (err error) {
	var eventRecordStreams []EventRecordStream
	queryRecord := EventRecordStream{
		RecordEvent: &config.RecordEvent{
			EventLevel: config.EventLevelHigh,
		},
		RecordStream: RecordStream{
			StreamPath: t.streamPath,
			Type:       t.Type,
		},
	}
	t.DB.Where(&queryRecord).Find(&eventRecordStreams) //搜索事件录像，且为重要事件（无法自动删除）
	if len(eventRecordStreams) > 0 {
		for _, recordStream := range eventRecordStreams {
			var unimportantEventRecordStreams []RecordStream
			query := `start_time <= ? and end_time >= ? and stream_path=? and type=?`
			t.DB.Where(query, recordStream.EndTime, recordStream.StartTime, t.streamPath, t.Type).Find(&unimportantEventRecordStreams)
			if len(unimportantEventRecordStreams) > 0 {
				for _, unimportantEventRecordStream := range unimportantEventRecordStreams {
					unimportantEventRecordStream.RecordLevel = config.EventLevelHigh
					t.DB.Save(&unimportantEventRecordStream)
				}
			}
		}
	}
	return
}
