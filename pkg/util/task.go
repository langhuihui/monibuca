package util

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"
)

const TraceLevel = slog.Level(-8)

var (
	ErrAutoStop     = errors.New("auto stop")
	ErrCallbackTask = errors.New("callback")
	ErrRetryRunOut  = errors.New("retry out")
	ErrTaskComplete = errors.New("complete")
	EmptyStart      = func() error { return nil }
	EmptyDispose    = func() {}
)

type (
	ITask interface {
		initTask(context.Context, ITask)
		getParent() *MarcoTask
		GetTask() *Task
		getSignal() reflect.Value
		Stop(error)
		StopReason() error
		start() error
		dispose()
		IsStopped() bool
		GetTaskType() string
		GetOwnerType() string
	}
	IMarcoTask interface {
		RangeSubTask(func(yield ITask) bool)
	}
	IChannelTask interface {
		tick(reflect.Value)
	}
	TaskStarter interface {
		Start() error
	}
	TaskDisposal interface {
		Dispose()
	}
	TaskBlock interface {
		Run() error
	}
	RetryConfig struct {
		MaxRetry      int
		RetryCount    int
		RetryInterval time.Duration
	}
	Task struct {
		ID        uint32
		StartTime time.Time
		*slog.Logger
		context.Context
		context.CancelCauseFunc
		retry                                      RetryConfig
		owner                                      string
		startHandler, runHandler                   func() error
		afterStartListeners, afterDisposeListeners []func()
		disposeHandler                             func()
		Description                                map[string]any
		startup, shutdown                          *Promise
		parent                                     *MarcoTask
		parentCtx                                  context.Context
	}
)

func (task *Task) SetRetry(maxRetry int, retryInterval time.Duration) {
	task.retry.MaxRetry = maxRetry
	task.retry.RetryInterval = retryInterval
}

func (task *Task) GetOwnerType() string {
	return task.owner
}

func (task *Task) GetTaskType() string {
	return "base"
}

func (task *Task) GetTask() *Task {
	return task
}

func (task *Task) getParent() *MarcoTask {
	return task.parent
}

func (task *Task) GetKey() uint32 {
	return task.ID
}

func (task *Task) WaitStarted() error {
	return task.startup.Await()
}

func (task *Task) WaitStopped() error {
	_ = task.WaitStarted()
	if task.shutdown == nil {
		return task.StopReason()
	}
	return task.shutdown.Await()
}

func (task *Task) Trace(msg string, fields ...any) {
	task.Log(task.Context, TraceLevel, msg, fields...)
}

func (task *Task) IsStopped() bool {
	return task.Err() != nil
}

func (task *Task) StopReason() error {
	return context.Cause(task.Context)
}

func (task *Task) Stop(err error) {
	if task.CancelCauseFunc != nil {
		if task.Logger != nil {
			task.Debug("task stop", "reason", err.Error(), "elapsed", time.Since(task.StartTime), "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
		}
		task.CancelCauseFunc(err)
	}
}

func (task *Task) OnStart(listener func()) {
	task.afterStartListeners = append(task.afterStartListeners, listener)
}

func (task *Task) OnDispose(listener func()) {
	task.afterDisposeListeners = append(task.afterDisposeListeners, listener)
}

func (task *Task) getSignal() reflect.Value {
	return reflect.ValueOf(task.Done())
}

func (task *Task) start() (err error) {
	task.StartTime = time.Now()
	if task.Logger != nil {
		task.Debug("task start", "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
	}
	for task.retry.MaxRetry < 0 || task.retry.RetryCount <= task.retry.MaxRetry {
		err = task.startHandler()
		if err == nil {
			break
		} else if task.IsStopped() {
			return task.StopReason()
		}
		task.retry.RetryCount++
		if task.Logger != nil {
			task.Warn(fmt.Sprintf("retry %d/%d", task.retry.RetryCount, task.retry.MaxRetry))
		}
		if delta := time.Since(task.StartTime); delta < task.retry.RetryInterval {
			time.Sleep(task.retry.RetryInterval - delta)
		}
	}
	task.startup.Fulfill(err)
	for _, listener := range task.afterStartListeners {
		listener()
	}
	if task.runHandler != nil {
		go task.run()
	}
	return
}

func (task *Task) dispose() {
	reason := task.StopReason()
	if task.Logger != nil {
		task.Debug("task dispose", "reason", reason, "taskId", task.ID, "taskType", task.GetTaskType(), "ownerType", task.GetOwnerType())
	}
	task.disposeHandler()
	task.shutdown.Fulfill(reason)
	for _, listener := range task.afterDisposeListeners {
		listener()
	}
}

func (task *Task) initTask(ctx context.Context, iTask ITask) {
	task.parentCtx = ctx
	task.Context, task.CancelCauseFunc = context.WithCancelCause(ctx)
	task.startup = NewPromise(task.Context)
	task.shutdown = NewPromise(context.Background())
	task.owner = reflect.TypeOf(iTask).Elem().Name()
	if v, ok := iTask.(TaskStarter); ok {
		task.startHandler = v.Start
	}
	if v, ok := iTask.(TaskDisposal); ok {
		task.disposeHandler = v.Dispose
	}
	if v, ok := iTask.(TaskBlock); ok {
		task.runHandler = v.Run
	}
}

func (task *Task) ResetRetryCount() {
	task.retry.RetryCount = 0
}

func (task *Task) run() {
	var err error
	retry := task.retry
	for !task.IsStopped() {
		if retry.MaxRetry < 0 || retry.RetryCount <= retry.MaxRetry {
			err = task.runHandler()
			if err == nil {
				task.Stop(ErrTaskComplete)
			} else {
				retry.RetryCount++
				if task.Logger != nil {
					task.Warn(fmt.Sprintf("retry %d/%d", retry.RetryCount, retry.MaxRetry))
				}
				if delta := time.Since(task.StartTime); delta < retry.RetryInterval {
					time.Sleep(retry.RetryInterval - delta)
				}
			}
		} else {
			if task.Logger != nil {
				task.Warn(fmt.Sprintf("max retry %d failed", retry.MaxRetry))
			}
			task.Stop(errors.Join(err, ErrRetryRunOut))
		}
	}
}
