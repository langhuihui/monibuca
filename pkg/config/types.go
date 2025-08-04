package config

import (
	"database/sql/driver"
	"fmt"
	"net/url"
	"time"

	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v3"
	"m7s.live/v5/pkg/util"
)

const (
	RelayModeRemux = "remux"
	RelayModeRelay = "relay"
	RelayModeMix   = "mix"

	RecordModeAuto  RecordMode = "auto"
	RecordModeEvent RecordMode = "event"
	RecordModeTest  RecordMode = "test"

	HookOnServerKeepAlive HookType = "server_keep_alive"
	HookOnPublishStart    HookType = "publish_start"
	HookOnPublishEnd      HookType = "publish_end"
	HookOnSubscribeStart  HookType = "subscribe_start"
	HookOnSubscribeEnd    HookType = "subscribe_end"
	HookOnPullStart       HookType = "pull_start"
	HookOnPullEnd         HookType = "pull_end"
	HookOnPushStart       HookType = "push_start"
	HookOnPushEnd         HookType = "push_end"
	HookOnRecordStart     HookType = "record_start"
	HookOnRecordEnd       HookType = "record_end"
	HookOnTransformStart  HookType = "transform_start"
	HookOnTransformEnd    HookType = "transform_end"
	HookOnSystemStart     HookType = "system_start"
	HookDefault           HookType = "default"

	EventLevelLow  EventLevel = "low"
	EventLevelHigh EventLevel = "high"

	AlarmStorageException        = 0x10010 // 存储异常
	AlarmStorageExceptionRecover = 0x10011 // 存储异常恢复
	AlarmPullOffline             = 0x10012 // 拉流异常，触发一次报警。
	AlarmPullRecover             = 0x10013 // 拉流恢复
	AlarmDiskSpaceFull           = 0x10014 // 磁盘空间满,磁盘占有率，超出最大磁盘空间使用率，触发报警。
	AlarmStartupRunning          = 0x10015 // 启动运行
	AlarmPublishOffline          = 0x10016 // 发布者异常，触发一次报警。
	AlarmPublishRecover          = 0x10017 // 发布者恢复
	AlarmSubscribeOffline        = 0x10018 // 订阅者异常，触发一次报警。
	AlarmSubscribeRecover        = 0x10019 // 订阅者恢复
	AlarmPushOffline             = 0x10020 // 推流异常，触发一次报警。
	AlarmPushRecover             = 0x10021 // 推流恢复
	AlarmTransformOffline        = 0x10022 // 转换异常，触发一次报警。
	AlarmTransformRecover        = 0x10023 // 转换恢复
	AlarmKeepAliveOnline         = 0x10024 // 保活正常，触发一次报警。
)

type (
	EventLevel = string
	RecordMode = string
	HookType   string
	Publish    struct {
		MaxCount          int             `default:"0" desc:"最大发布者数量"` // 最大发布者数量
		PubAudio          bool            `default:"true" desc:"是否发布音频"`
		PubVideo          bool            `default:"true" desc:"是否发布视频"`
		KickExist         bool            `desc:"是否踢掉已经存在的发布者"`                                             // 是否踢掉已经存在的发布者
		PublishTimeout    time.Duration   `default:"10s" desc:"发布无数据超时"`                                    // 发布无数据超时
		WaitCloseTimeout  time.Duration   `desc:"延迟自动关闭（等待重连）"`                                             // 延迟自动关闭（等待重连）
		DelayCloseTimeout time.Duration   `desc:"延迟自动关闭（无订阅时）"`                                             // 延迟自动关闭（无订阅时）
		IdleTimeout       time.Duration   `desc:"空闲(无订阅)超时"`                                                // 空闲(无订阅)超时
		PauseTimeout      time.Duration   `default:"30s" desc:"暂停超时时间"`                                     // 暂停超时
		BufferTime        time.Duration   `desc:"缓冲时长，0代表取最近关键帧"`                                           // 缓冲长度(单位：秒)，0代表取最近关键帧
		Speed             float64         `desc:"发送速率"`                                                     // 发送速率，0 为不限速
		Scale             float64         `default:"1" desc:"缩放倍数"`                                         // 缩放倍数
		MaxFPS            int             `default:"60" desc:"最大FPS"`                                       // 最大FPS
		Key               string          `desc:"发布鉴权key"`                                                  // 发布鉴权key
		RingSize          util.Range[int] `default:"20-1024" desc:"RingSize范围"`                             // 缓冲区大小范围
		RelayMode         string          `default:"remux" desc:"转发模式" enum:"remux:转格式,relay:纯转发,mix:混合转发"` // 转发模式
		PubType           string          `default:"server" desc:"发布类型"`                                    // 发布类型
		Dump              bool
	}
	Subscribe struct {
		MaxCount        int           `default:"0" desc:"最大订阅者数量"` // 最大订阅者数量
		SubAudio        bool          `default:"true" desc:"是否订阅音频"`
		SubVideo        bool          `default:"true" desc:"是否订阅视频"`
		BufferTime      time.Duration `desc:"缓冲时长,从缓冲时长的关键帧开始播放"`
		SubMode         int           `desc:"订阅模式" enum:"0:实时模式,1:首屏后不进行追赶"`                // 0，实时模式：追赶发布者进度，在播放首屏后等待发布者的下一个关键帧，然后跳到该帧。1、首屏后不进行追赶。2、从缓冲最大的关键帧开始播放，也不追赶，需要发布者配置缓存长度
		SyncMode        int           `default:"1" desc:"同步模式" enum:"0:采用时间戳同步,1:采用写入时间同步"` // 0，采用时间戳同步，1，采用写入时间同步
		IFrameOnly      bool          `desc:"只要关键帧"`                                        // 只要关键帧
		WaitTimeout     time.Duration `default:"10s" desc:"等待流超时时间"`                        // 等待流超时
		WaitTrack       string        `default:"video" desc:"等待轨道" enum:"audio:等待音频,video:等待视频,all:等待全部"`
		WriteBufferSize int           `desc:"写缓冲大小"`   // 写缓冲大小
		Key             string        `desc:"订阅鉴权key"` // 订阅鉴权key
		SubType         string        `desc:"订阅类型"`    // 订阅类型
	}
	HTTPValues map[string][]string
	Pull       struct {
		URL           string        `desc:"拉流地址"`
		Loop          int           `desc:"拉流循环次数,-1:无限循环"`          // 拉流循环次数，-1 表示无限循环
		MaxRetry      int           `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重拉,0 表示不自动重拉，-1 表示无限重拉，高于0 的数代表最大重拉次数
		RetryInterval time.Duration `default:"5s" desc:"重试间隔"`       // 重试间隔
		Proxy         string        `desc:"代理地址"`                    // 代理地址
		Header        HTTPValues
		Args          HTTPValues `gorm:"-:all"`              // 拉流参数
		TestMode      int        `desc:"测试模式,0:关闭,1:只拉流不发布"` // 测试模式
	}
	Push struct {
		URL           string        `desc:"推送地址"`                    // 推送地址
		MaxRetry      int           `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重推,0 表示不自动重推，-1 表示无限重推，高于0 的数代表最大重推次数
		RetryInterval time.Duration `default:"5s" desc:"重试间隔"`       // 重试间隔
		Proxy         string        `desc:"代理地址"`                    // 代理地址
		Header        HTTPValues
	}
	RecordEvent struct {
		EventId        string
		BeforeDuration uint32     `json:"beforeDuration" desc:"事件前缓存时长" gorm:"comment:事件前缓存时长;default:30000"`
		AfterDuration  uint32     `json:"afterDuration" desc:"事件后缓存时长" gorm:"comment:事件后缓存时长;default:30000"`
		EventDesc      string     `json:"eventDesc" desc:"事件描述" gorm:"type:varchar(255);comment:事件描述"`
		EventLevel     EventLevel `json:"eventLevel" desc:"事件级别" gorm:"type:varchar(255);comment:事件级别,high表示重要事件，无法删除且表示无需自动删除,low表示非重要事件,达到自动删除时间后，自动删除;default:'low'"`
		EventName      string     `json:"eventName" desc:"事件名称" gorm:"type:varchar(255);comment:事件名称"`
	}
	Record struct {
		Mode     RecordMode    `json:"mode" desc:"事件类型,auto=连续录像模式，event=事件录像模式" gorm:"type:varchar(255);comment:事件类型,auto=连续录像模式，event=事件录像模式;default:'auto'"`
		Type     string        `desc:"录制类型"`                         // 录制类型 mp4、flv、hls、hlsv7
		FilePath string        `desc:"录制文件路径"`                       // 录制文件路径
		Fragment time.Duration `desc:"分片时长"`                         // 分片时长
		RealTime bool          `desc:"是否实时录制"`                       // 是否实时录制
		Append   bool          `desc:"是否追加录制"`                       // 是否追加录制
		Event    *RecordEvent  `json:"event" desc:"事件录像配置" gorm:"-"` // 事件录像配置
	}
	TransfromOutput struct {
		Target     string `desc:"转码目标"` // 转码目标
		StreamPath string
		Conf       any
	}
	Transform struct {
		Input  any
		Output []TransfromOutput
	}
	OnPublish struct {
		Push      map[Regexp]Push
		Record    map[Regexp]Record
		Transform map[Regexp]Transform
	}
	OnSubscribe struct {
		Pull      map[Regexp]Pull
		Transform map[Regexp]Transform
	}
	Webhook struct {
		URL            string            // Webhook 地址
		Method         string            `default:"POST"` // HTTP 方法
		Headers        map[string]string // 自定义请求头
		TimeoutSeconds int               `default:"5"`     // 超时时间(秒)
		RetryTimes     int               `default:"3"`     // 重试次数
		RetryInterval  time.Duration     `default:"1s"`    // 重试间隔
		Interval       int               `default:"60"`    // 保活间隔(秒)
		SaveAlarm      bool              `default:"false"` // 是否保存告警到数据库
	}
	Common struct {
		PublicIP   string
		PublicIPv6 string
		LogLevel   string `default:"info" enum:"trace:跟踪,debug:调试,info:信息,warn:警告,error:错误"` //日志级别
		EnableAuth bool   `desc:"启用鉴权"`                                                      //启用鉴权
		Publish
		Subscribe
		HTTP
		Quic
		TCP
		UDP
		Hook      map[HookType]Webhook
		Pull      map[string]Pull
		Transform map[string]Transform
		OnSub     OnSubscribe
		OnPub     OnPublish
		DB
	}
	ICommonConf interface {
		GetCommonConf() *Common
	}
)

func NewPublish() *Publish {
	p := &Publish{}
	defaults.SetDefaults(p)
	p.RingSize = util.Range[int]{20, 1024}
	return p
}

func (p *Record) GetRecordConfig() *Record {
	return p
}

func (v *HTTPValues) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal yaml value: %v", value)
	}
	return yaml.Unmarshal(bytes, v)
}

func (v HTTPValues) Value() (driver.Value, error) {
	return yaml.Marshal(v)
}

func (v HTTPValues) Get(key string) string {
	return url.Values(v).Get(key)
}

func (v HTTPValues) DeepClone() (ret HTTPValues) {
	ret = make(HTTPValues)
	for k, v := range v {
		ret[k] = append([]string(nil), v...)
	}
	return
}

func (r *TransfromOutput) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		// If it's a string, assign it to Target
		return node.Decode(&r.Target)
	}

	if node.Kind == yaml.MappingNode {
		var conf map[string]any
		if err := node.Decode(&conf); err != nil {
			return err
		}
		var normal bool
		if conf["target"] != nil {
			r.Target = conf["target"].(string)
			normal = true
		}
		if conf["streampath"] != nil {
			r.StreamPath = conf["streampath"].(string)
			normal = true
		}
		if conf["conf"] != nil {
			r.Conf = conf["conf"]
			normal = true
		}
		if !normal {
			r.Conf = conf
		}
		return nil
	}

	return fmt.Errorf("unsupported node kind: %v", node.Kind)
}
