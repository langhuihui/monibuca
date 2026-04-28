package pkg

import (
	"gorm.io/gorm"
	"time"
)

// RecordPlanStream 录制计划流信息模型
type RecordPlanStream struct {
	StreamPath string `json:"stream_path" gorm:"primaryKey;type:varchar(255)"`
	RecordType string `json:"record_type" gorm:"primaryKey;type:varchar(255)"`
	PlanID     uint   `json:"plan_id" gorm:"type:bigint;not null;index"` // 录制计划ID
	Fragment   string `json:"fragment" gorm:"type:varchar(255)"`
	FilePath   string `json:"file_path" gorm:"type:varchar(255)"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Enable     bool `json:"enable" gorm:"default:false"` // 是否启用
}

// TableName 设置表名
func (RecordPlanStream) TableName() string {
	return "record_plans_streams"
}

// ScopeStreamPathLike 模糊查询 StreamPath
func ScopeStreamPathLike(streamPath string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if streamPath != "" {
			return db.Where("record_plans_streams.stream_path LIKE ?", "%"+streamPath+"%")
		}
		return db
	}
}

// ScopeOrderByCreatedAtDesc 按创建时间倒序
func ScopeOrderByCreatedAtDesc() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order("record_plans_streams.created_at DESC")
	}
}

// ScopeRecordPlanID 按录制计划ID查询
func ScopeRecordPlanID(recordPlanID uint) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if recordPlanID > 0 {
			return db.Where(&RecordPlanStream{PlanID: recordPlanID})
		}
		return db
	}
}
