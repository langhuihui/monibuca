package task

type CallBackTask struct {
	Task
	startHandler   func() error
	disposeHandler func()
}

func (t *CallBackTask) GetTaskType() TaskType {
	return TASK_TYPE_CALL
}

func (t *CallBackTask) Start() error {
	return t.startHandler()
}

func (t *CallBackTask) Dispose() {
	if t.disposeHandler != nil {
		t.disposeHandler()
	}
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
