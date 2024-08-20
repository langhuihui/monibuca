package monitor

import "time"

type Session struct {
	ID                 uint32 `gorm:"primarykey"`
	StartTime, EndTime time.Time
}
