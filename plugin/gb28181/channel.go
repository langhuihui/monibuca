package plugin_gb28181pro

import (
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

type RecordRequest struct {
	SN, SumNum  int
	ReceivedNum int // 已接收的记录数
	Response    []gb28181.Message
	*util.Promise
}

func (r *RecordRequest) GetKey() int {
	return r.SN
}

// AddResponse 添加响应并检查是否完成
func (r *RecordRequest) AddResponse(msg gb28181.Message) bool {
	r.Response = append(r.Response, msg)
	r.ReceivedNum += msg.RecordList.Num
	// 当接收到的记录数等于总数时，表示接收完成
	return r.ReceivedNum >= msg.SumNum
}

// PresetRequest 预置位请求结构体
type PresetRequest struct {
	SN       int
	Response []gb28181.PresetItem
	*util.Promise
}

func (r *PresetRequest) GetKey() int {
	return r.SN
}

type Channel struct {
	PullProxyTask       *PullProxy   // 拉流任务
	Device              *Device      // 所属设备
	State               atomic.Int32 // 通道状态,0:空闲,1:正在invite,2:正在播放/对讲
	GpsTime             time.Time    // gps时间
	Longitude, Latitude string       // 经度
	RecordReqs          util.Collection[int, *RecordRequest]
	PresetReqs          util.Collection[int, *PresetRequest] // 预置位请求集合
	*slog.Logger
	*gb28181.DeviceChannel
}

func (c *Channel) GetKey() string {
	return c.ID
}

type PullProxy struct {
	task.TickTask
	m7s.BasePullProxy
	deviceId, channelId string
	devices             *task.WorkCollection[string, *Device]
}

func NewPullProxy() m7s.IPullProxy {
	return &PullProxy{}
}

func (p *PullProxy) GetKey() uint {
	return p.PullProxyConfig.ID
}

func (p *PullProxy) Start() error {
	streamPaths := strings.Split(p.GetStreamPath(), "/")
	p.deviceId, p.channelId = streamPaths[0], streamPaths[1]
	p.devices = &p.Plugin.GetHandler().(*GB28181Plugin).devices
	return p.TickTask.Start()
}

func (p *PullProxy) Tick(any) {
	if device, ok := p.devices.Get(p.deviceId); ok {
		if _, ok := device.channels.Get(p.deviceId + "_" + p.channelId); ok {
			p.ChangeStatus(m7s.PullProxyStatusOnline)
			return
		}
	}
	p.ChangeStatus(m7s.PullProxyStatusOffline)
}
