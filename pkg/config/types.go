package config

import (
	"fmt"
	"m7s.live/m7s/v5/pkg/util"
	"regexp"
	"strings"
	"time"
)

type PublishConfig interface {
	GetPublishConfig() *Publish
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
	MaxCount          int             `default:"0" desc:"最大发布者数量"` // 最大发布者数量
	PubAudio          bool            `default:"true" desc:"是否发布音频"`
	PubVideo          bool            `default:"true" desc:"是否发布视频"`
	KickExist         bool            `desc:"是否踢掉已经存在的发布者"`                 // 是否踢掉已经存在的发布者
	PublishTimeout    time.Duration   `default:"10s" desc:"发布无数据超时"`        // 发布无数据超时
	WaitCloseTimeout  time.Duration   `desc:"延迟自动关闭（等待重连）"`                 // 延迟自动关闭（等待重连）
	DelayCloseTimeout time.Duration   `desc:"延迟自动关闭（无订阅时）"`                 // 延迟自动关闭（无订阅时）
	IdleTimeout       time.Duration   `desc:"空闲(无订阅)超时"`                    // 空闲(无订阅)超时
	PauseTimeout      time.Duration   `default:"30s" desc:"暂停超时时间"`         // 暂停超时
	BufferTime        time.Duration   `desc:"缓冲时长，0代表取最近关键帧"`               // 缓冲长度(单位：秒)，0代表取最近关键帧
	Speed             float64         `default:"0" desc:"倍速"`               // 倍速，0 为不限速
	Key               string          `desc:"发布鉴权key"`                      // 发布鉴权key
	RingSize          util.Range[int] `default:"20-1024" desc:"RingSize范围"` // 缓冲区大小范围
	Dump              bool
}

func (c *Publish) GetPublishConfig() *Publish {
	return c
}

type Subscribe struct {
	MaxCount        int           `default:"0" desc:"最大订阅者数量"` // 最大订阅者数量
	SubAudio        bool          `default:"true" desc:"是否订阅音频"`
	SubVideo        bool          `default:"true" desc:"是否订阅视频"`
	BufferTime      time.Duration `desc:"缓冲时长,从缓冲时长的关键帧开始播放"`
	SubMode         int           `desc:"订阅模式" enum:"0:实时模式,1:首屏后不进行追赶"`    // 0，实时模式：追赶发布者进度，在播放首屏后等待发布者的下一个关键帧，然后跳到该帧。1、首屏后不进行追赶。2、从缓冲最大的关键帧开始播放，也不追赶，需要发布者配置缓存长度
	SyncMode        int           `desc:"同步模式" enum:"0:采用时间戳同步,1:采用写入时间同步"` // 0，采用时间戳同步，1，采用写入时间同步
	IFrameOnly      bool          `desc:"只要关键帧"`                            // 只要关键帧
	WaitTimeout     time.Duration `default:"10s" desc:"等待流超时时间"`            // 等待流超时
	WriteBufferSize int           `desc:"写缓冲大小"`                            // 写缓冲大小
	Key             string        `desc:"订阅鉴权key"`                          // 订阅鉴权key
	Internal        bool          `default:"false" desc:"是否内部订阅"`           // 是否内部订阅
}

func (c *Subscribe) GetSubscribeConfig() *Subscribe {
	return c
}

type Pull struct {
	RePull       int               `desc:"断开后自动重试次数,0:不重试,-1:无限重试"` // 断开后自动重拉,0 表示不自动重拉，-1 表示无限重拉，高于0 的数代表最大重拉次数
	EnableRegexp bool              `desc:"是否启用正则表达式"`               // 是否启用正则表达式
	PullOnStart  map[string]string `desc:"启动时拉流的列表"`                // 启动时拉流的列表
	PullOnSub    map[string]string `desc:"订阅时自动拉流的列表"`              // 订阅时自动拉流的列表
	Proxy        string            `desc:"代理地址"`                    // 代理地址
}

func (p *Pull) GetPullConfig() *Pull {
	return p
}

func (p *Pull) CheckPullOnStart(streamPath string) string {
	// p.PullOnStartLocker.RLock()
	// defer p.PullOnStartLocker.RUnlock()
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
	// p.PullOnSubLocker.RLock()
	// defer p.PullOnSubLocker.RUnlock()
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

type Common struct {
	PublicIP   string
	LogLevel   string `default:"info" enum:"trace:跟踪,debug:调试,info:信息,warn:警告,error:错误"` //日志级别
	EnableAuth bool   `desc:"启用鉴权"`                                                      //启用鉴权
	Publish
	Subscribe
	HTTP
	Quic
	TCP
	UDP
	Pull
	Push
}

type ICommonConf interface {
	GetCommonConf() *Common
}
