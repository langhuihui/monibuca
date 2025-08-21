package pkg

import (
	"gorm.io/gorm"
)

// RecordPlan 录制计划模型
type RecordPlan struct {
	gorm.Model
	Name   string `json:"name" gorm:"default:''"`
	Plan   string `json:"plan" gorm:"type:varchar(255)"`
	Enable bool   `json:"enable" gorm:"default:false"` // 是否启用
}

func (r *RecordPlan) GetKey() uint {
	return r.ID
}
