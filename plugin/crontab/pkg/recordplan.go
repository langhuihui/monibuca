package pkg

import (
	"gorm.io/gorm"
)

// RecordPlan 录制计划模型
type RecordPlan struct {
	gorm.Model
	Name    string `json:"name" gorm:"default:''"`
	Plan    string `json:"plan" gorm:"type:text"`
	Enabled bool   `json:"enabled" gorm:"default:true"`
}
