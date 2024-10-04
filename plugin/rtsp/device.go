package plugin_rtsp

import (
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/pkg/util"
	. "m7s.live/m7s/v5/plugin/rtsp/pkg"
)

type RTSPDevice struct {
	task.TickTask
	conn   Stream
	device *m7s.Device
	plugin *RTSPPlugin
}

func (d *RTSPDevice) Start() (err error) {
	d.conn.NetConnection = &NetConnection{
		MemoryAllocator: util.NewScalableMemoryAllocator(1 << 12),
		UserAgent:       "monibuca" + m7s.Version,
	}
	d.conn.Logger = d.plugin.Logger
	return d.TickTask.Start()
}

func (d *RTSPDevice) GetTickInterval() time.Duration {
	return time.Second * 5
}

func (d *RTSPDevice) Pull() {
	d.plugin.Pull(d.device.GetStreamPath(), config.Pull{URL: d.device.PullURL, MaxRetry: -1, RetryInterval: time.Second * 5})
}

func (d *RTSPDevice) Tick(any) {
	if d.device.Status != m7s.DeviceStatusOnline {
		err := d.conn.Connect(d.device.PullURL)
		if err != nil {
			return
		}
		d.device.ChangeStatus(m7s.DeviceStatusOnline)
	}
	err := d.conn.Options()
	if err != nil {
		d.device.ChangeStatus(m7s.DeviceStatusOffline)
	}
}

func (d *RTSPDevice) Dispose() {
	d.device.ChangeStatus(m7s.DeviceStatusOffline)
	d.TickTask.Dispose()
}
