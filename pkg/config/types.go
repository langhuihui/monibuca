package config

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

type PublishConfig interface {
	GetPublishConfig() Publish
}

type SubscribeConfig interface {
	GetSubscribeConfig() *Subscribe
}
type PullConfig interface {
	GetPullConfig() *Pull
}

type PushConfig interface {
	GetPushConfig() *Push
}

type Publish struct {
	PubAudio          bool          `default:"true" desc:"是否发布音频"`
	PubVideo          bool          `default:"true" desc:"是否发布视频"`
	KickExist         bool          `desc:"是否踢掉已经存在的发布者"`                     // 是否踢掉已经存在的发布者
	PublishTimeout    time.Duration `default:"10s" desc:"发布无数据超时"`            // 发布无数据超时
	WaitCloseTimeout  time.Duration `desc:"延迟自动关闭（等待重连）"`                     // 延迟自动关闭（等待重连）
	DelayCloseTimeout time.Duration `desc:"延迟自动关闭（无订阅时）"`                     // 延迟自动关闭（无订阅时）
	IdleTimeout       time.Duration `desc:"空闲(无订阅)超时"`                        // 空闲(无订阅)超时
	PauseTimeout      time.Duration `default:"30s" desc:"暂停超时时间"`             // 暂停超时
	BufferTime        time.Duration `desc:"缓冲长度(单位：秒)，0代表取最近关键帧"`             // 缓冲长度(单位：秒)，0代表取最近关键帧
	SpeedLimit        time.Duration `default:"500ms" desc:"速度限制最大等待时间,0则不等待"` //速度限制最大等待时间
	Key               string        `desc:"发布鉴权key"`                          // 发布鉴权key
	SecretArgName     string        `default:"secret" desc:"发布鉴权参数名"`         // 发布鉴权参数名
	ExpireArgName     string        `default:"expire" desc:"发布鉴权失效时间参数名"`     // 发布鉴权失效时间参数名
	RingSize          string        `default:"256-1024" desc:"缓冲范围"`          // 初始缓冲区大小
}

func (c Publish) GetPublishConfig() Publish {
	return c
}

type Subscribe struct {
	SubAudio        bool          `default:"true" desc:"是否订阅音频"`
	SubVideo        bool          `default:"true" desc:"是否订阅视频"`
	SubVideoArgName string        `default:"vts" desc:"定订阅的视频轨道参数名"`                     // 指定订阅的视频轨道参数名
	SubAudioArgName string        `default:"ats" desc:"指定订阅的音频轨道参数名"`                    // 指定订阅的音频轨道参数名
	SubDataArgName  string        `default:"dts" desc:"指定订阅的数据轨道参数名"`                    // 指定订阅的数据轨道参数名
	SubModeArgName  string        `desc:"指定订阅的模式参数名"`                                    // 指定订阅的模式参数名
	SubAudioTracks  []string      `desc:"指定订阅的音频轨道"`                                     // 指定订阅的音频轨道
	SubVideoTracks  []string      `desc:"指定订阅的视频轨道"`                                     // 指定订阅的视频轨道
	SubDataTracks   []string      `desc:"指定订阅的数据轨道"`                                     // 指定订阅的数据轨道
	SubMode         int           `desc:"订阅模式" enum:"0:实时模式,1:首屏后不进行追赶,2:从缓冲最大的关键帧开始播放"` // 0，实时模式：追赶发布者进度，在播放首屏后等待发布者的下一个关键帧，然后跳到该帧。1、首屏后不进行追赶。2、从缓冲最大的关键帧开始播放，也不追赶，需要发布者配置缓存长度
	SyncMode        int           `desc:"同步模式" enum:"0:采用时间戳同步,1:采用写入时间同步"`              // 0，采用时间戳同步，1，采用写入时间同步
	IFrameOnly      bool          `desc:"只要关键帧"`                                         // 只要关键帧
	WaitTimeout     time.Duration `default:"10s" desc:"等待流超时时间"`                         // 等待流超时
	WriteBufferSize int           `desc:"写缓冲大小"`                                         // 写缓冲大小
	Key             string        `desc:"订阅鉴权key"`                                       // 订阅鉴权key
	SecretArgName   string        `default:"secret" desc:"订阅鉴权参数名"`                      // 订阅鉴权参数名
	ExpireArgName   string        `default:"expire" desc:"订阅鉴权失效时间参数名"`                  // 订阅鉴权失效时间参数名
	Internal        bool          `default:"false" desc:"是否内部订阅"`                        // 是否内部订阅
}

func (c *Subscribe) GetSubscribeConfig() *Subscribe {
	return c
}

type Pull struct {
	RePull            int               `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重拉,0 表示不自动重拉，-1 表示无限重拉，高于0 的数代表最大重拉次数
	EnableRegexp      bool              `desc:"是否启用正则表达式"`               // 是否启用正则表达式
	PullOnStart       map[string]string `desc:"启动时拉流的列表"`                // 启动时拉流的列表
	PullOnSub         map[string]string `desc:"订阅时自动拉流的列表"`              // 订阅时自动拉流的列表
	Proxy             string            `desc:"代理地址"`                    // 代理地址
	PullOnSubLocker   sync.RWMutex      `yaml:"-" json:"-"`
	PullOnStartLocker sync.RWMutex      `yaml:"-" json:"-"`
}

func (p *Pull) GetPullConfig() *Pull {
	return p
}

func (p *Pull) CheckPullOnStart(streamPath string) string {
	p.PullOnStartLocker.RLock()
	defer p.PullOnStartLocker.RUnlock()
	if p.PullOnStart == nil {
		return ""
	}
	url, ok := p.PullOnStart[streamPath]
	if !ok && p.EnableRegexp {
		for k, url := range p.PullOnStart {
			if r, err := regexp.Compile(k); err != nil {
				if group := r.FindStringSubmatch(streamPath); group != nil {
					for i, value := range group {
						url = strings.Replace(url, fmt.Sprintf("$%d", i), value, -1)
					}
					return url
				}
			}
			return ""
		}
	}
	return url
}

func (p *Pull) CheckPullOnSub(streamPath string) string {
	p.PullOnSubLocker.RLock()
	defer p.PullOnSubLocker.RUnlock()
	if p.PullOnSub == nil {
		return ""
	}
	url, ok := p.PullOnSub[streamPath]
	if !ok && p.EnableRegexp {
		for k, url := range p.PullOnSub {
			if r, err := regexp.Compile(k); err == nil {
				if group := r.FindStringSubmatch(streamPath); group != nil {
					for i, value := range group {
						url = strings.Replace(url, fmt.Sprintf("$%d", i), value, -1)
					}
					return url
				}
			}
			return ""
		}
	}
	return url
}

type Push struct {
	EnableRegexp bool              `desc:"是否启用正则表达式"`               // 是否启用正则表达式
	RePush       int               `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重推,0 表示不自动重推，-1 表示无限重推，高于0 的数代表最大重推次数
	PushList     map[string]string `desc:"自动推流列表"`                  // 自动推流列表
	Proxy        string            `desc:"代理地址"`                    // 代理地址
}

func (p *Push) GetPushConfig() *Push {
	return p
}

func (p *Push) AddPush(url string, streamPath string) {
	if p.PushList == nil {
		p.PushList = make(map[string]string)
	}
	p.PushList[streamPath] = url
}

func (p *Push) CheckPush(streamPath string) string {
	url, ok := p.PushList[streamPath]
	if !ok && p.EnableRegexp {
		for k, url := range p.PushList {
			if r, err := regexp.Compile(k); err == nil {
				if group := r.FindStringSubmatch(streamPath); group != nil {
					for i, value := range group {
						url = strings.Replace(url, fmt.Sprintf("$%d", i), value, -1)
					}
					return url
				}
			}
			return ""
		}
	}
	return url
}

type Console struct {
	Server        string `default:"console.monibuca.com:44944" desc:"远程控制台地址"` //远程控制台地址
	Secret        string `desc:"远程控制台密钥"`                                      //远程控制台密钥
	PublicAddr    string `desc:"远程控制台公网地址"`                                    //公网地址，提供远程控制台访问的地址，不配置的话使用自动识别的地址
	PublicAddrTLS string `desc:"远程控制台公网TLS地址"`
}

type Engine struct {
	EnableAVCC          bool          `default:"true" desc:"启用AVCC格式，rtmp、http-flv协议使用"`                 //启用AVCC格式，rtmp、http-flv协议使用
	EnableRTP           bool          `default:"true" desc:"启用RTP格式，rtsp、webrtc等协议使用"`                   //启用RTP格式，rtsp、webrtc等协议使用
	EnableSubEvent      bool          `default:"true" desc:"启用订阅事件,禁用可以提高性能"`                            //启用订阅事件,禁用可以提高性能
	EnableAuth          bool          `default:"true" desc:"启用鉴权"`                                       //启用鉴权
	LogLang             string        `default:"zh" desc:"日志语言" enum:"zh:中文,en:英文"`                      //日志语言
	LogLevel            string        `default:"info" enum:"trace:跟踪,debug:调试,info:信息,warn:警告,error:错误"` //日志级别
	SettingDir          string        `default:".m7s" desc:""`
	EventBusSize        int           `default:"10" desc:"事件总线大小"`      //事件总线大小
	PulseInterval       time.Duration `default:"5s" desc:"心跳事件间隔"`      //心跳事件间隔
	DisableAll          bool          `default:"false" desc:"禁用所有插件"`   //禁用所有插件
	RTPReorderBufferLen int           `default:"50" desc:"RTP重排序缓冲区长度"` //RTP重排序缓冲区长度
	PoolSize            int           `desc:"内存池大小"`                    //内存池大小
}

type Common struct {
	Publish
	Subscribe
	HTTP
	Quic
	TCP
	Pull
	Push
}

type ICommonConf interface {
	GetCommonConf() *Common
}
