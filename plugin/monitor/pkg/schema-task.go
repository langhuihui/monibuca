package monitor

import (
	"time"
)

type Task struct {
	ID          uint32 `gorm:"primarykey"`
	CreatedAt   time.Time
	StartTime   time.Time
	OwnerType   string
	TaskType    byte
	Description string
	Reason      string
}

func (i *Task) GetKey() uint32 {
	return i.ID
}
