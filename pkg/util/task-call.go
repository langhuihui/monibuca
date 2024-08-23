package util

type CallBackTask struct {
	Task
}

func (t *CallBackTask) GetTaskType() TaskType {
	return TASK_TYPE_CALL
}

func CreateTaskByCallBack(start func() error, dispose func()) ITask {
	var task CallBackTask
	task.startHandler = func() error {
		err := start()
		if err == nil && dispose == nil {
			err = ErrTaskComplete
		}
		return err
	}
	task.disposeHandler = dispose
	return &task
}
