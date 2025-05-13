package plugin_gb28181pro

import (
	"time"

	"m7s.live/v5/pkg/task"
)

// AlarmSubscribeTask 报警订阅任务
type AlarmSubscribeTask struct {
	task.TickTask
	device *Device
}

// NewAlarmSubscribeTask 创建新的报警订阅任务
func NewAlarmSubscribeTask(device *Device) *AlarmSubscribeTask {
	device.AlarmSubscribeTask = &AlarmSubscribeTask{
		device: device,
	}
	return device.AlarmSubscribeTask
}

// GetTickInterval 获取定时间隔
func (p *AlarmSubscribeTask) GetTickInterval() time.Duration {
	// 如果设备配置了报警订阅周期，则使用设备配置的周期，否则使用默认值3600秒
	if p.device.SubscribeAlarm > 0 {
		return time.Second * time.Duration(p.device.SubscribeAlarm)
	}
	return time.Second * 3600
}

// Tick 定时执行的方法
func (p *AlarmSubscribeTask) Tick(any) {
	response, err := p.device.subscribeAlarm()
	if err != nil {
		p.Error("subAlarm", "err", err)
	} else {
		p.Debug("subAlarm", "response", response.String())
	}
}
