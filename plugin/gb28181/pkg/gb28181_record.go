package gb28181

import "time"

// GB28181Record GB28181录像缓存表，用于避免重复下载相同时间段的录像
type GB28181Record struct {
	DownloadId string    `gorm:"primaryKey"` // 格式：{deviceId}_{channelId}_{startTime}_{endTime}
	FilePath   string    // MP4文件路径（绝对路径）
	Status     string    // completed/failed
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (GB28181Record) TableName() string {
	return "gb28181_record"
}

// GetKey 实现 Collection 接口
func (r *GB28181Record) GetKey() string {
	return r.DownloadId
}
