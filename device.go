package m7s

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-ping/ping"
	"gorm.io/gorm"
	"m7s.live/pro/pkg/config"
	"m7s.live/pro/pkg/task"
)

const (
	DeviceStatusOffline byte = iota
	DeviceStatusOnline
	DeviceStatusPulling
	DeviceStatusDisabled
)

type (
	IDevice interface {
		Pull()
	}
	Device struct {
		server               *Server `gorm:"-:all"`
		task.Work            `gorm:"-:all" yaml:"-"`
		ID                   uint           `gorm:"primarykey"`
		CreatedAt, UpdatedAt time.Time      `yaml:"-"`
		DeletedAt            gorm.DeletedAt `gorm:"index" yaml:"-"`
		Name                 string
		StreamPath           string
		PullOnStart          bool
		config.Pull          `gorm:"embedded;embeddedPrefix:pull_"`
		config.Record        `gorm:"embedded;embeddedPrefix:record_"`
		ParentID             uint
		Type                 string
		Status               byte
		Description          string
		RTT                  time.Duration
		Handler              IDevice `gorm:"-:all" yaml:"-"`
	}
	DeviceManager struct {
		task.Manager[uint32, *Device]
	}
	DeviceTask struct {
		task.TickTask
		Device *Device
		Plugin *Plugin
	}
	HTTPDevice struct {
		DeviceTask
		url *url.URL
	}
)

func (d *Device) GetStreamPath() string {
	if d.StreamPath == "" {
		return fmt.Sprintf("device/%s/%d", d.Type, d.ID)
	}
	return d.StreamPath
}

func (d *Device) Start() (err error) {
	for plugin := range d.server.Plugins.Range {
		if devicePlugin, ok := plugin.handler.(IDevicePlugin); ok && strings.EqualFold(d.Type, plugin.Meta.Name) {
			deviceTask := devicePlugin.OnDeviceAdd(d)
			if deviceTask != nil {
				d.AddTask(deviceTask)
			}
		}
	}
	return
}

func (d *Device) ChangeStatus(status byte) {
	if d.Status == status {
		return
	}
	from := d.Status
	d.Info("device status changed", "from", from, "to", status)
	d.Status = status
	d.Update()
	switch status {
	case DeviceStatusOnline:
		if d.PullOnStart && from == DeviceStatusOffline {
			d.Handler.Pull()
		}
	case DeviceStatusPulling:
		if from == DeviceStatusOnline && d.FilePath != "" {
			if mp4Plugin, ok := d.server.Plugins.Get("MP4"); ok {
				mp4Plugin.Record(d.GetStreamPath(), d.Record)
			}
		}
	}
}

func (d *Device) Update() {
	if d.server.DB != nil {
		d.server.DB.Save(d)
	}
}

func (d *HTTPDevice) Start() (err error) {
	d.url, err = url.Parse(d.Device.URL)
	return
}

func (d *HTTPDevice) GetTickInterval() time.Duration {
	return time.Second * 10
}

func (d *HTTPDevice) Tick(any) {
	pinger, err := ping.NewPinger(d.url.Hostname())
	if err != nil {
		d.Device.ChangeStatus(DeviceStatusOffline)
		return
	}
	pinger.Count = 1
	err = pinger.Run() // Blocks until finished.
	if err != nil {
		d.Device.ChangeStatus(DeviceStatusOffline)
		return
	}
	stats := pinger.Statistics()
	d.Device.RTT = stats.AvgRtt
	d.Device.ChangeStatus(DeviceStatusOnline)
}

func (d *DeviceTask) Dispose() {
	d.Device.ChangeStatus(DeviceStatusOffline)
	d.TickTask.Dispose()
}

func (d *DeviceTask) Pull() {
	d.Plugin.handler.Pull(d.Device.GetStreamPath(), d.Device.Pull)
}
