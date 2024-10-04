package m7s

import (
	"fmt"

	"gorm.io/gorm"
	"m7s.live/m7s/v5/pkg/task"
)

const (
	DeviceStatusOffline byte = iota
	DeviceStatusOnline
	DeviceStatusPulling
)

type (
	IDevice interface {
		Pull()
	}
	Device struct {
		server    *Server `gorm:"-:all"`
		task.Work `gorm:"-:all"`
		gorm.Model
		Name     string
		PullURL  string
		ParentID uint
		Type     string
		Status   byte
		Handler  IDevice `gorm:"-:all"`
	}
	DeviceManager struct {
		task.Manager[uint32, *Device]
	}
)

func (d *Device) GetStreamPath() string {
	return fmt.Sprintf("device/%s/%d", d.Type, d.ID)
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
