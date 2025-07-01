package m7s

import (
	"time"
)

// AlarmInfo 报警信息实体，用于存储到数据库
type AlarmInfo struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`            // 主键，自增ID
	ServerInfo string    `gorm:"type:varchar(255);not null" json:"server_info"` // 服务器信息
	StreamName string    `gorm:"type:varchar(255);index" json:"stream_name"`    // 流名称
	StreamPath string    `gorm:"type:varchar(500)" json:"stream_path"`          // 流的streampath
	AlarmDesc  string    `gorm:"type:varchar(500);not null" json:"alarm_desc"`  // 报警描述
	AlarmType  int       `gorm:"not null;index" json:"alarm_type"`              // 报警类型（对应之前定义的常量）
	IsSent     bool      `gorm:"default:false" json:"is_sent"`                  // 是否已成功发送
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`              // 创建时间,报警时间
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`              // 更新时间
}

// TableName 指定表名
func (AlarmInfo) TableName() string {
	return "alarm_info"
}
