package plugin_onvif

import (
	"fmt"
	"sync"
	"time"

	"m7s.live/v5/pkg/util"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
)

const VIRTUAL_IFACE = "virtual"

var (
	_ = m7s.InstallPlugin[OnvifPlugin](m7s.PluginMeta{})
)

type OnvifPlugin struct {
	m7s.Plugin
	DiscoverInterval int  `default:"3" desc:"设备发现间隔（秒），0表示不自动发现"`
	AutoPull         bool `default:"false" desc:"是否自动拉流"`
	AutoAdd          bool `default:"false" desc:"是否自动添加发现的设备"`
	Interfaces       []struct {
		InterfaceName string `desc:"网卡名称"`
		Username      string `desc:"用户名"`
		Password      string `desc:"密码"`
	} `desc:"网卡配置"`
	Devices []struct {
		IP       string `desc:"设备IP"`
		Username string `desc:"用户名"`
		Password string `desc:"密码"`
	} `desc:"设备配置"`
}

type InterfaceCollection struct {
	util.Collection[string, *DeviceStatus]
	iface string
}

func (c *InterfaceCollection) GetKey() string {
	return c.iface
}

// OnvifTimerTask 定时任务结构体
type OnvifTimerTask struct {
	task.TickTask
	plugin *OnvifPlugin
}

// GetTickInterval 设置定时间隔
func (t *OnvifTimerTask) GetTickInterval() time.Duration {
	return time.Duration(t.plugin.DiscoverInterval) * time.Second
}

// Tick 执行定时任务
func (t *OnvifTimerTask) Tick(any) {
	deviceList.discoveryDevice()
	if t.plugin.AutoPull {
		deviceList.AutoPullStream()
	}
}

//func (p *OnvifPlugin) OnEvent(event any) {
//	switch e := event.(type) {
//	case pkg.IStreamEvent:
//		if e.Type() == pkg.SEclose {
//			stream := e.Target().Path
//			device := deviceList.GetDeviceByStreamPath(stream)
//			if device != nil {
//				device.Stream = ""
//			}
//		}
//	}
//}

func (p *OnvifPlugin) Start() (err error) {
	// 检查配置参数
	if p.DiscoverInterval < 0 {
		p.Error("invalid discover interval",
			"interval", p.DiscoverInterval,
			"valid_range", ">=0",
		)
		return fmt.Errorf("invalid discover interval: %d, valid range is >=0", p.DiscoverInterval)
	}

	// 初始化设备列表
	deviceList.Data = &util.Collection[string, *InterfaceCollection]{
		Items: make([]*InterfaceCollection, 0),
		L:     &sync.RWMutex{},
	}
	deviceList.plugin = p
	virtualIface := &InterfaceCollection{
		Collection: util.Collection[string, *DeviceStatus]{
			Items: make([]*DeviceStatus, 0),
			L:     &sync.RWMutex{},
		},
		iface: VIRTUAL_IFACE,
	}
	deviceList.Data.Add(virtualIface)
	preprocessAuth(p, authCfg)

	// 如果设置了发现间隔，启动设备发现任务
	if p.DiscoverInterval > 0 {
		// 立即执行一次发现
		deviceList.discoveryDevice()
		if p.AutoPull {
			deviceList.AutoPullStream()
		}

		// 添加定时任务
		p.AddTask(&OnvifTimerTask{
			plugin: p,
		})
	}

	p.Info("onvif plugin initialized",
		"discover_interval", p.DiscoverInterval,
		"auto_pull", p.AutoPull,
		"auto_add", p.AutoAdd,
	)

	return nil
}

func preprocessAuth(conf *OnvifPlugin, c *AuthConfig) {
	for _, i := range conf.Interfaces {
		c.Interfaces[i.InterfaceName] = deviceAuth{
			Username: i.Username,
			Password: i.Password,
		}
	}
	for _, d := range conf.Devices {
		c.Devices[d.IP] = deviceAuth{
			Username: d.Username,
			Password: d.Password,
		}
	}
}
