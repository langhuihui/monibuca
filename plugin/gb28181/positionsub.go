package plugin_gb28181pro

import (
	"time"

	"m7s.live/v5/pkg/task"
)

// PositionSubscribeTask 位置订阅任务
type PositionSubscribeTask struct {
	task.TickTask
	device *Device
}

// NewPositionSubscribeTask 创建新的位置订阅任务
func NewPositionSubscribeTask(device *Device) *PositionSubscribeTask {
	return &PositionSubscribeTask{
		device: device,
	}
}

// GetTickInterval 获取定时间隔
func (p *PositionSubscribeTask) GetTickInterval() time.Duration {
	// 如果设备配置了位置订阅周期，则使用设备配置的周期，否则使用默认值3600秒
	if p.device.SubscribePosition > 0 {
		return time.Second * time.Duration(p.device.SubscribePosition)
	}
	return time.Second * 3600
}

// Tick 定时执行的方法
func (p *PositionSubscribeTask) Tick(any) {
	// 执行位置订阅，使用设备配置的位置间隔，如果未配置则使用默认值6
	interval := 6
	if p.device.PositionInterval > 0 {
		interval = p.device.PositionInterval
	}
	
	response, err := p.device.subscribePosition(interval)
	if err != nil {
		p.Error("subPosition", "err", err)
	} else {
		p.Debug("subPosition", "response", response.String())
	}
}
