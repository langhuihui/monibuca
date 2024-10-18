package plugin_rtsp

import (
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	. "m7s.live/m7s/v5/plugin/rtsp/pkg"
)

type RTSPDevice struct {
	m7s.DeviceTask
	conn Stream
}

func (d *RTSPDevice) Start() (err error) {
	d.conn.NetConnection = &NetConnection{
		MemoryAllocator: util.NewScalableMemoryAllocator(1 << 12),
		UserAgent:       "monibuca" + m7s.Version,
	}
	d.conn.Logger = d.Plugin.Logger
	return d.TickTask.Start()
}

func (d *RTSPDevice) GetTickInterval() time.Duration {
	return time.Second * 5
}

func (d *RTSPDevice) Tick(any) {
	if d.Device.Status != m7s.DeviceStatusOnline {
		err := d.conn.Connect(d.Device.PullConfig.URL)
		if err != nil {
			return
		}
		d.Device.ChangeStatus(m7s.DeviceStatusOnline)
	}
	t := time.Now()
	err := d.conn.Options()
	d.Device.RTT = time.Since(t)
	if err != nil {
		d.Device.ChangeStatus(m7s.DeviceStatusOffline)
	}
}
