package m7s

import "m7s.live/m7s/v5/pkg/task"

type (
	Device struct {
		task.Work
	}
	DeviceManager struct {
		task.Manager[uint32, *Device]
	}
)
