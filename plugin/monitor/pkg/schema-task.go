package monitor

import (
	"time"
)

type Task struct {
	ID                          uint `gorm:"primarykey"`
	SessionID, TaskID, ParentID uint32
	StartTime, EndTime          time.Time
	OwnerType                   string
	TaskType                    byte
	Description                 string
	Reason                      string
}
