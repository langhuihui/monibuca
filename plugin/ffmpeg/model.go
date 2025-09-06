package ffmpeg

import (
	"gorm.io/gorm"
)

type FFmpegProcess struct {
	Arguments string // 参数
	gorm.Model
	Description string // 描述
	Status      string `gorm:"-"`             // 状态: stopped, running, error
	PID         int    `gorm:"-"`             // 进程 ID
	AutoStart   bool   `gorm:"default:false"` // 是否自动启动
}

// TableName 设置表名
func (FFmpegProcess) TableName() string {
	return "ffmpeg_processes"
}
