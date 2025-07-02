package m7s

import (
	"time"
)

// AlarmInfo 报警信息实体，用于存储到数据库
type AlarmInfo struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`           // 主键，自增ID
	ServerInfo string    `gorm:"type:varchar(255);not null" json:"serverInfo"` // 服务器信息
	StreamName string    `gorm:"type:varchar(255);index" json:"streamName"`    // 流名称
	StreamPath string    `gorm:"type:varchar(500)" json:"streamPath"`          // 流的streampath
	AlarmName  string    `gorm:"type:varchar(255);not null" json:"alarmName"`  // 报警名称
	AlarmDesc  string    `gorm:"type:varchar(500);not null" json:"alarmDesc"`  // 报警描述
	AlarmType  int       `gorm:"not null;index" json:"alarmType"`              // 报警类型（对应之前定义的常量）
	IsSent     bool      `gorm:"default:false" json:"isSent"`                  // 是否已成功发送
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"createdAt"`              // 创建时间,报警时间
	UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updatedAt"`              // 更新时间
	FilePath   string    `gorm:"type:varchar(255)" json:"filePath"`            // 文件路径
}

// TableName 指定表名
func (AlarmInfo) TableName() string {
	return "alarm_info"
}
