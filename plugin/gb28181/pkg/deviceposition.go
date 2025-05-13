package gb28181

import (
	"time"
)

// DevicePosition 设备位置信息表
type DevicePosition struct {
	ID         uint      `gorm:"primaryKey"`
	DeviceID   string    `gorm:"index"` // 设备国标编号
	GpsTime    time.Time // GPS时间
	Longitude  float64   // 经度
	Latitude   float64   // 纬度
	CreateTime time.Time // 记录创建时间
}

func (DevicePosition) TableName() string {
	return "gb28181_device_position"
}
