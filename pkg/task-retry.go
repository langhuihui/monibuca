package pkg

import (
	"fmt"
	"reflect"
	"time"
)

type RetryTask struct {
	Task
	MaxRetry      int
	RetryCount    int
	RetryInterval time.Duration
}

func (task *RetryTask) start() (signal reflect.Value, err error) {
	task.StartTime = time.Now()
	for !task.parent.IsStopped() {
		err = task.startHandler()
		if task.Logger != nil {
			task.Debug("task start", "taskId", task.ID)
		}
		task.startup.Fulfill(err)
		if err == nil {
			return
		}
		if task.MaxRetry < 0 || task.RetryCount < task.MaxRetry {
			task.RetryCount++
			if delta := time.Since(task.StartTime); delta < task.RetryInterval {
				time.Sleep(task.RetryInterval - delta)
			}
			if task.Logger != nil {
				task.Warn(fmt.Sprintf("retry %d/%d", task.RetryCount, task.MaxRetry))
			}
			task.init(task.parentCtx)
		} else {
			if task.Logger != nil {
				task.Warn(fmt.Sprintf("max retry %d failed", task.MaxRetry))
			}
			task.Stop(ErrRetryRunOut)
			return reflect.ValueOf(task.Done()), nil
		}
	}
	return
}
