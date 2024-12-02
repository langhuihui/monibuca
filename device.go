package m7s

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
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
		DeletedAt                      gorm.DeletedAt `yaml:"-"`
		Name                           string
		StreamPath                     string
		PullOnStart, Audio, StopOnIdle bool
		config.Pull                    `gorm:"embedded;embeddedPrefix:pull_"`
		config.Record                  `gorm:"embedded;embeddedPrefix:record_"`
		ParentID                       uint
		Type                           string
		Status                         byte
		Description                    string
		RTT                            time.Duration
		Handler                        IDevice `gorm:"-:all" yaml:"-"`
	}
	DeviceManager struct {
		task.Manager[uint, *Device]
	}
	DeviceTask struct {
		task.TickTask
		Device *Device
		Plugin *Plugin
	}
	HTTPDevice struct {
		DeviceTask
		tcpAddr *net.TCPAddr
		url     *url.URL
	}
)

func (d *Device) GetKey() uint {
	return d.ID
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
			deviceTask := devicePlugin.OnDeviceAdd(d)
			if deviceTask == nil {
				continue
			}
			if deviceTask, ok := deviceTask.(IDevice); ok {
				d.Handler = deviceTask
			}
			if t, ok := deviceTask.(task.ITask); ok {
				if ticker, ok := t.(task.IChannelTask); ok {
					t.OnStart(func() {
						ticker.Tick(nil)
					})
				}
				d.AddTask(t)
			} else {
				d.ChangeStatus(DeviceStatusOnline)
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
	}
}

func (d *Device) Update() {
	if d.server.DB != nil {
		d.server.DB.Omit("deleted_at").Save(d)
	}
}

func (d *DeviceTask) Dispose() {
	d.Device.ChangeStatus(DeviceStatusOffline)
	d.TickTask.Dispose()
	d.Plugin.Server.Streams.Call(func() error {
		if stream, ok := d.Plugin.Server.Streams.Get(d.Device.GetStreamPath()); ok {
			stream.Stop(task.ErrStopByUser)
		}
		return nil
	})
}

func (d *DeviceTask) Pull() {
	var pubConf = d.Plugin.config.Publish
	pubConf.PubAudio = d.Device.Audio
	pubConf.DelayCloseTimeout = util.Conditional(d.Device.StopOnIdle, time.Second*5, 0)
	d.Plugin.handler.Pull(d.Device.GetStreamPath(), d.Device.Pull, &pubConf)
}

func (d *HTTPDevice) Start() (err error) {
	d.url, err = url.Parse(d.Device.URL)
	if err != nil {
		return
	}
	if ips, err := net.LookupIP(d.url.Hostname()); err != nil {
		return err
	} else if len(ips) == 0 {
		return fmt.Errorf("no IP found for host: %s", d.url.Hostname())
	} else {
		d.tcpAddr, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(ips[0].String(), d.url.Port()))
		if err != nil {
			return err
		}
		if d.tcpAddr.Port == 0 {
			if d.url.Scheme == "https" || d.url.Scheme == "wss" {
				d.tcpAddr.Port = 443
			} else {
				d.tcpAddr.Port = 80
			}
		}
	}
	return d.DeviceTask.Start()
}

func (d *HTTPDevice) GetTickInterval() time.Duration {
	return time.Second * 10
}

func (d *HTTPDevice) Tick(any) {
	startTime := time.Now()
	conn, err := net.DialTCP("tcp", nil, d.tcpAddr)
	if err != nil {
		d.Device.ChangeStatus(DeviceStatusOffline)
		return
	}
	conn.Close()
	d.Device.RTT = time.Since(startTime)
	d.Device.ChangeStatus(DeviceStatusOnline)
}
