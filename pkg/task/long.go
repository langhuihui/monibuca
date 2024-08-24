package task

type MarcoLongTask struct {
	MarcoTask
}

func (m *MarcoLongTask) keepalive() bool {
	return true
}

func (*MarcoLongTask) GetTaskType() TaskType {
	return TASK_TYPE_LONG_MACRO
}
