package gb28181

import (
	"github.com/emiago/sipgo/sip"
	"log/slog"
	"m7s.live/m7s/v5/pkg/util"
	"time"
)

type DeviceStatus string

const (
	DeviceRegisterStatus DeviceStatus = "REGISTER"
	DeviceRecoverStatus  DeviceStatus = "RECOVER"
	DeviceOnlineStatus   DeviceStatus = "ONLINE"
	DeviceOfflineStatus  DeviceStatus = "OFFLINE"
	DeviceAlarmedStatus  DeviceStatus = "ALARMED"
)

type Device struct {
	ID              string
	Name            string
	Manufacturer    string
	Model           string
	Owner           string
	RegisterTime    time.Time
	UpdateTime      time.Time
	LastKeepaliveAt time.Time
	Status          DeviceStatus
	SN              int
	Addr            sip.Addr
	SipIP           string //设备对应网卡的服务器ip
	MediaIP         string //设备对应网卡的服务器ip
	NetAddr         string
	channels        util.Collection[string, *Channel]
	subscriber      struct {
		CallID  string
		Timeout time.Time
	}
	lastSyncTime time.Time
	GpsTime      time.Time //gps时间
	Longitude    string    //经度
	Latitude     string    //纬度
	*slog.Logger
}

func (d *Device) GetKey() string {
	return d.ID
}
