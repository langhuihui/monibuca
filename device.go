package m7s

import (
	"fmt"
	"time"

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
	Device struct {
		server                         *Server `gorm:"-:all"`
		task.Work                      `gorm:"-:all" yaml:"-"`
		ID                             uint           `gorm:"primarykey"`
		CreatedAt, UpdatedAt           time.Time      `yaml:"-"`
		DeletedAt                      gorm.DeletedAt `gorm:"index" yaml:"-"`
		Name                           string
		StreamPath                     string
		PullURL                        string
		PullOnStart, StopOnIdle, Audio bool
		Record                         config.Record `gorm:"embedded;embeddedPrefix:record_"`
		ParentID                       uint
		Type                           string
		Status                         byte
		Description                    string
		Handler                        IDevice `gorm:"-:all" yaml:"-"`
	}
	DeviceManager struct {
		task.Manager[uint32, *Device]
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
		if devicePlugin, ok := plugin.handler.(IDevicePlugin); ok {
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
	d.Info("device status changed", "from", d.Status, "to", status)
	d.Status = status
	d.Update()
}

func (d *Device) Update() {
	if d.server.DB != nil {
		d.server.DB.Save(d)
	}
}
