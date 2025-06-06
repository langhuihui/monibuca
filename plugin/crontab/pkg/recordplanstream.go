package pkg

import (
	"time"

	"gorm.io/gorm"
)

// RecordPlanStream 录制计划流信息模型
type RecordPlanStream struct {
	RecordPlanID uint      `json:"record_plan_id" gorm:"primaryKey;autoIncrement:false"`
	StreamPath   string    `json:"stream_path" gorm:"primaryKey;type:varchar(255)"`
	Fragment     string    `json:"fragment" gorm:"type:text"`
	FilePath     string    `json:"file_path" gorm:"type:varchar(255)"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
			return db.Where(&RecordPlanStream{RecordPlanID: recordPlanID})
		}
		return db
	}
}
