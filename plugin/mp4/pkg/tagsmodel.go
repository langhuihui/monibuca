package mp4

import (
	"time"

	"gorm.io/gorm"
)

// TagModel 标签模型
type TagModel struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	TagName    string         `json:"tagName" gorm:"type:varchar(255);comment:标签名称"`
	StreamPath string         `json:"streamPath" gorm:"type:varchar(255);comment:流路径"`
	TagTime    time.Time      `json:"tagTime" gorm:"comment:标签时间"`
	CreatedAt  time.Time      `json:"createdAt" gorm:"comment:创建时间"`
	UpdatedAt  time.Time      `json:"updatedAt" gorm:"comment:修改时间"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"deletedAt,omitempty"`
}

// TableName 指定数据库表名
func (d *TagModel) TableName() string {
	return "record_tag"
}
