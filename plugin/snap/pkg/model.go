package snap

import (
	"time"
)

// SnapRecord 截图记录
type SnapRecord struct {
	ID         uint      `gorm:"primarykey"`
	StreamName string    `gorm:"index"` // 流名称
	SnapMode   int       // 截图模式
	SnapTime   time.Time `gorm:"index"` // 截图时间
	SnapPath   string    // 截图路径
	CreatedAt  time.Time
}

// TableName 指定表名
func (SnapRecord) TableName() string {
	return "snap_records"
}
