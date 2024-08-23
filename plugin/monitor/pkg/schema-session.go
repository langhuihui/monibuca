package monitor

import "time"

type Session struct {
	ID                 uint32 `gorm:"primarykey"`
	PID                int
	Args               string
	StartTime, EndTime time.Time
}
