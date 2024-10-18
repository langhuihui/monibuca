package m7s

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-ping/ping"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
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
	DevicePullConfig struct {
		config.Pull
		config.Record
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
		PullConfig           DevicePullConfig
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

// 实现 sql.Scanner 接口，Scan 将 value 扫描至 Jsonb
func (j *DevicePullConfig) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}
	return yaml.Unmarshal(bytes, j)
}

// 实现 driver.Valuer 接口，Value 返回 json value
func (j DevicePullConfig) Value() (driver.Value, error) {
	return yaml.Marshal(j)
}

func (d *Device) GetStreamPath() string {
	if d.StreamPath == "" {
		return fmt.Sprintf("device/%s/%d", d.Type, d.ID)
	}
	return d.StreamPath
}

func (d *Device) Start() (err error) {
	for plugin := range d.server.Plugins.Range {
		if devicePlugin, ok := plugin.handler.(IDevicePlugin); ok && strings.EqualFold(d.Type, plugin.Meta.Name) {
			task := devicePlugin.OnDeviceAdd(d)
			if task != nil {
				d.AddTask(task)
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
		if from == DeviceStatusOnline && d.PullConfig.FilePath != "" {
			if mp4Plugin, ok := d.server.Plugins.Get("MP4"); ok {
				mp4Plugin.Record(d.GetStreamPath(), d.PullConfig.Record)
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
	d.url, err = url.Parse(d.Device.PullConfig.URL)
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
	d.Plugin.handler.Pull(d.Device.GetStreamPath(), d.Device.PullConfig.Pull)
}
