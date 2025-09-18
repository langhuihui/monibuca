package plugin_gb28181pro

import (
	"github.com/emiago/sipgo/sip"
	"time"

	"m7s.live/v5/pkg/task"
)

// CatalogSubscribeTask 目录订阅任务
type CatalogSubscribeTask struct {
	task.TickTask
	device *Device
}

// NewCatalogSubscribeTask 创建新的目录订阅任务
func NewCatalogSubscribeTask(device *Device) *CatalogSubscribeTask {
	return &CatalogSubscribeTask{
		device: device,
	}
}

// GetTickInterval 获取定时间隔
func (c *CatalogSubscribeTask) GetTickInterval() time.Duration {
	// 如果设备配置了订阅周期，则使用设备配置的周期，否则使用默认值3600秒
	if c.device.SubscribeCatalog > 0 {
		return time.Second * time.Duration(c.device.SubscribeCatalog)
	}
	return time.Second * 3600
}

// Tick 定时执行的方法
func (c *CatalogSubscribeTask) Tick(any) {
	var response *sip.Response
	var err error
	if c.device.SubscribeCatalog > 0 {
		response, err = c.device.subscribeCatalog()
	} else {
		response, err = c.device.unSubscribeCatalog()
	}
	if err != nil {
		c.Error("subCatalog", "err", err)
	} else {
		c.Debug("subCatalog", "response", response.String())
	}
}
