package m7s

import (
	"gorm.io/gorm"
	"m7s.live/m7s/v5/pkg/task"
)

const (
	DeviceStatusOffline byte = iota
	DeviceStatusOnline
	DeviceStatusPulling
)

const (
	DeviceTypeGroup byte = iota
	DeviceTypeGB
	DeviceTypeRTSP
	DeviceTypeRTMP
	DeviceTypeWebRTC
)

type (
	Device struct {
		task.Work `gorm:"-:all"`
		gorm.Model
		ParentID  uint
		Type      byte
		StreamURL string
		Status    byte
	}
	DeviceManager struct {
		task.Manager[uint32, *Device]
	}
)
