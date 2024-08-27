package task

type Work struct {
	Job
}

func (m *Work) keepalive() bool {
	return true
}

func (*Work) GetTaskType() TaskType {
	return TASK_TYPE_Work
}
