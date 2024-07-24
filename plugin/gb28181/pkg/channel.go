package gb28181

import (
	"log/slog"
	"sync/atomic"
	"time"
)

type ChannelStatus string

const (
	ChannelOnStatus  ChannelStatus = "ON"
	ChannelOffStatus ChannelStatus = "OFF"
)

type Channel struct {
	Device    *Device      // 所属设备
	State     atomic.Int32 // 通道状态,0:空闲,1:正在invite,2:正在播放/对讲
	LiveSubSP string       // 实时子码流，通过rtsp
	GpsTime   time.Time    // gps时间
	Longitude string       // 经度
	Latitude  string       // 纬度
	*slog.Logger
	ChannelInfo
}

func (c *Channel) GetKey() string {
	return c.DeviceID
}

type ChannelInfo struct {
	DeviceID     string // 通道ID
	ParentID     string
	Name         string
	Manufacturer string
	Model        string
	Owner        string
	CivilCode    string
	Address      string
	Port         int
	Parental     int
	SafetyWay    int
	RegisterWay  int
	Secrecy      int
	Status       ChannelStatus
}
