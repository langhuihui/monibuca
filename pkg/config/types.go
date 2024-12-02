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
)

type (
	Publish struct {
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
		Speed             float64         `default:"0" desc:"倍速"`                                           // 倍速，0 为不限速
		Key               string          `desc:"发布鉴权key"`                                                  // 发布鉴权key
		RingSize          util.Range[int] `default:"20-1024" desc:"RingSize范围"`                             // 缓冲区大小范围
		RelayMode         string          `default:"remux" desc:"转发模式" enum:"remux:转格式,relay:纯转发,mix:混合转发"` // 转发模式
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
		WriteBufferSize int           `desc:"写缓冲大小"`                                        // 写缓冲大小
		Key             string        `desc:"订阅鉴权key"`                                      // 订阅鉴权key
		Internal        bool          `default:"false" desc:"是否内部订阅"`                       // 是否内部订阅
	}
	HTTPValus map[string][]string
	Pull      struct {
		URL           string        `desc:"拉流地址"`
		MaxRetry      int           `default:"-1" desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重拉,0 表示不自动重拉，-1 表示无限重拉，高于0 的数代表最大重拉次数
		RetryInterval time.Duration `default:"5s" desc:"重试间隔"`                    // 重试间隔
		Proxy         string        `desc:"代理地址"`                                 // 代理地址
		Header        HTTPValus
		Args          HTTPValus `gorm:"-:all"` // 拉流参数
	}
	Push struct {
		URL           string        `desc:"推送地址"`                    // 推送地址
		MaxRetry      int           `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重推,0 表示不自动重推，-1 表示无限重推，高于0 的数代表最大重推次数
		RetryInterval time.Duration `default:"5s" desc:"重试间隔"`       // 重试间隔
		Proxy         string        `desc:"代理地址"`                    // 代理地址
		Header        HTTPValus
	}
	Record struct {
		FilePath string        `desc:"录制文件路径"` // 录制文件路径
		Fragment time.Duration `desc:"分片时长"`   // 分片时长
		Append   bool          `desc:"是否追加录制"` // 是否追加录制
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

func (v *HTTPValus) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal yaml value: %v", value)
	}
	return yaml.Unmarshal(bytes, v)
}

func (v HTTPValus) Value() (driver.Value, error) {
	return yaml.Marshal(v)
}

func (v HTTPValus) Get(key string) string {
	return url.Values(v).Get(key)
}

func (v HTTPValus) DeepClone() (ret HTTPValus) {
	ret = make(HTTPValus)
	for k, v := range v {
		ret[k] = append([]string(nil), v...)
	}
	return
}
